import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import {
  createHash,
  createHmac,
  generateKeyPairSync,
  createPublicKey,
  sign as signMessage,
  verify as verifySignature,
} from "node:crypto";
import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const contractsDirectory = path.resolve(scriptDirectory, "..");
const repositoryDirectory = path.resolve(contractsDirectory, "..");
const platformDirectory = path.join(contractsDirectory, "platform");
const fixtureDirectory = path.join(contractsDirectory, "fixtures", "platform");

const readJson = async (filePath) =>
  JSON.parse(await readFile(filePath, "utf8"));

const sha256 = (value) =>
  createHash("sha256").update(value).digest("hex");

const clone = (value) => JSON.parse(JSON.stringify(value));

function assertUnpairedSurrogatesAbsent(value) {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(index + 1);
      assert(
        next >= 0xdc00 && next <= 0xdfff,
        "JCS input contains an unpaired high surrogate",
      );
      index += 1;
    } else {
      assert(
        code < 0xdc00 || code > 0xdfff,
        "JCS input contains an unpaired low surrogate",
      );
    }
  }
}

function jcsCanonicalize(value) {
  if (value === null) {
    return "null";
  }
  if (typeof value === "string") {
    assertUnpairedSurrogatesAbsent(value);
    return JSON.stringify(value);
  }
  if (typeof value === "boolean") {
    return value ? "true" : "false";
  }
  if (typeof value === "number") {
    assert(Number.isFinite(value), "JCS rejects non-finite numbers");
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) {
    return `[${value.map(jcsCanonicalize).join(",")}]`;
  }
  assert(
    typeof value === "object" &&
      [Object.prototype, null].includes(Object.getPrototypeOf(value)),
    "JCS accepts only JSON objects",
  );
  const entries = Object.keys(value)
    .sort()
    .map((key) => {
      assertUnpairedSurrogatesAbsent(key);
      assert(value[key] !== undefined, "JCS rejects undefined values");
      return `${JSON.stringify(key)}:${jcsCanonicalize(value[key])}`;
    });
  return `{${entries.join(",")}}`;
}

const requestDigestBoundFields = [
  "schema_version",
  "method",
  "route_template",
  "path",
  "query",
  "body",
  "user_id",
  "trading_account_id",
];

function canonicalRequestDigest(input) {
  const bound = {};
  for (const field of requestDigestBoundFields) {
    assert(
      Object.hasOwn(input, field),
      `request digest missing bound field ${field}`,
    );
    bound[field] = input[field];
  }
  assert(
    /^[A-Z]+$/.test(bound.method),
    "request digest method must already be uppercase",
  );
  return sha256(jcsCanonicalize(bound));
}

function isoMilliseconds(value) {
  const parsed = Date.parse(value);
  assert(Number.isFinite(parsed), `invalid timestamp ${value}`);
  return parsed;
}

function parseStrictJson(raw) {
  let cursor = 0;
  const skipWhitespace = () => {
    while (/[ \t\r\n]/u.test(raw[cursor] ?? "")) cursor += 1;
  };
  const parseString = () => {
    assert.equal(raw[cursor], '"', "expected JSON string");
    const start = cursor;
    cursor += 1;
    while (cursor < raw.length) {
      if (raw[cursor] === "\\") {
        cursor += 2;
        continue;
      }
      if (raw[cursor] === '"') {
        cursor += 1;
        return JSON.parse(raw.slice(start, cursor));
      }
      cursor += 1;
    }
    assert.fail("unterminated JSON string");
  };
  const parseValue = () => {
    skipWhitespace();
    if (raw[cursor] === "{") return parseObject();
    if (raw[cursor] === "[") return parseArray();
    if (raw[cursor] === '"') return parseString();
    const match = raw.slice(cursor).match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/u);
    assert(match, `invalid JSON value at byte ${cursor}`);
    cursor += match[0].length;
    return JSON.parse(match[0]);
  };
  const parseArray = () => {
    const values = [];
    cursor += 1;
    skipWhitespace();
    if (raw[cursor] === "]") {
      cursor += 1;
      return values;
    }
    while (true) {
      values.push(parseValue());
      skipWhitespace();
      if (raw[cursor] === "]") {
        cursor += 1;
        return values;
      }
      assert.equal(raw[cursor], ",", "expected array comma");
      cursor += 1;
    }
  };
  const parseObject = () => {
    const value = Object.create(null);
    const keys = new Set();
    cursor += 1;
    skipWhitespace();
    if (raw[cursor] === "}") {
      cursor += 1;
      return value;
    }
    while (true) {
      skipWhitespace();
      const key = parseString();
      assert(!keys.has(key), `duplicate JSON object key ${key}`);
      assert(
        !["__proto__", "constructor", "prototype"].includes(key),
        `unsafe JSON object key ${key}`,
      );
      keys.add(key);
      skipWhitespace();
      assert.equal(raw[cursor], ":", "expected object colon");
      cursor += 1;
      value[key] = parseValue();
      skipWhitespace();
      if (raw[cursor] === "}") {
        cursor += 1;
        return value;
      }
      assert.equal(raw[cursor], ",", "expected object comma");
      cursor += 1;
    }
  };
  const value = parseValue();
  skipWhitespace();
  assert.equal(cursor, raw.length, "trailing JSON bytes");
  return value;
}

function containsCaseInsensitiveField(value, deniedFields) {
  if (Array.isArray(value)) {
    return value.some((item) =>
      containsCaseInsensitiveField(item, deniedFields),
    );
  }
  if (value === null || typeof value !== "object") return false;
  return Object.keys(value).some(
    (key) =>
      deniedFields.has(key.toLowerCase()) ||
      containsCaseInsensitiveField(value[key], deniedFields),
  );
}

function enrollmentStateFromFixture(fixture) {
  return {
    identities: new Map(
      fixture.identity_directory.map((identity) => [
        identity.identifier,
        {
          userId: identity.user_id,
          passwordVerifierSha256: identity.password_verifier_sha256,
        },
      ]),
    ),
    challenges: new Map(),
    devices: new Map(),
    sessions: new Map(),
    refreshFamilies: new Map(),
    publicKeyOwners: new Map(),
    auditEvents: new Set(),
    notifications: new Set(),
    outboxRecords: new Set(),
  };
}

function startEnrollment(state, input, wire) {
  const identity = state.identities.get(input.identifier);
  const passwordValid =
    identity !== undefined &&
    input.password_attempt_sha256 === identity.passwordVerifierSha256;
  if (identity === undefined || !passwordValid) {
    return { wire, persisted: false };
  }
  state.challenges.set(wire.subject_handle, {
    userId: identity.userId,
    wire: clone(wire),
    attempted: false,
  });
  state.auditEvents.add(`challenge-audit:${wire.subject_handle}`);
  state.outboxRecords.add(`challenge-outbox:${wire.subject_handle}`);
  return { wire, persisted: true };
}

function enrollmentProofIsValid(wire, completion) {
  try {
    const rawPublicKey = Buffer.from(
      completion.candidate_public_key.slice("ed25519-public:".length),
      "hex",
    );
    if (
      completion.candidate_public_key !==
        `ed25519-public:${rawPublicKey.toString("hex")}` ||
      rawPublicKey.length !== 32 ||
      wire.candidate_public_key_fingerprint !==
        `ed25519:${sha256(rawPublicKey)}`
    ) {
      return false;
    }
    const publicKey = createPublicKey({
      key: Buffer.concat([
        Buffer.from("302a300506032b6570032100", "hex"),
        rawPublicKey,
      ]),
      format: "der",
      type: "spki",
    });
    const canonical = jcsCanonicalize(wire);
    const signedBytes = Buffer.concat([
      Buffer.from(wire.domain, "utf8"),
      Buffer.from([0]),
      Buffer.from(canonical, "utf8"),
    ]);
    const signature = Buffer.from(
      completion.signature.slice("ed25519-signature:".length),
      "hex",
    );
    return (
      completion.signature ===
        `ed25519-signature:${signature.toString("hex")}` &&
      signature.length === 64 &&
      verifySignature(null, signedBytes, publicKey, signature)
    );
  } catch {
    return false;
  }
}

function completeEnrollment(state, completion) {
  const challenge = state.challenges.get(completion.subject_handle);
  if (challenge === undefined) return "REJECT_NO_SERVER_STATE";
  if (challenge.attempted) {
    state.auditEvents.add(`replay-audit:${completion.subject_handle}`);
    state.outboxRecords.add(`replay-outbox:${completion.subject_handle}`);
    return "REJECT_REPLAY";
  }
  challenge.attempted = true;
  const wire = challenge.wire;
  const bindingValid =
    completion.nonce === wire.nonce &&
    completion.purpose === wire.purpose &&
    completion.candidate_public_key_fingerprint ===
      wire.candidate_public_key_fingerprint &&
    enrollmentProofIsValid(wire, completion) &&
    isoMilliseconds(completion.completed_at) <
      isoMilliseconds(wire.expires_at);
  if (!bindingValid) {
    state.auditEvents.add(`invalid-proof-audit:${completion.subject_handle}`);
    state.outboxRecords.add(`invalid-proof-outbox:${completion.subject_handle}`);
    return "CONSUMED_INVALID_PROOF";
  }
  const existingOwner = state.publicKeyOwners.get(
    wire.candidate_public_key_fingerprint,
  );
  if (existingOwner !== undefined) {
    const outcome =
      existingOwner === challenge.userId
        ? "REJECT_DUPLICATE_KEY"
        : "REJECT_CROSS_OWNER_REBIND";
    state.auditEvents.add(`${outcome}-audit:${completion.subject_handle}`);
    state.outboxRecords.add(`${outcome}-outbox:${completion.subject_handle}`);
    return outcome;
  }
  if (wire.purpose === "REPLACEMENT_DEVICE") {
    for (const record of state.devices.values()) {
      if (record.userId === challenge.userId) record.active = false;
    }
    for (const record of state.sessions.values()) {
      if (record.userId === challenge.userId) record.active = false;
    }
    for (const record of state.refreshFamilies.values()) {
      if (record.userId === challenge.userId) record.active = false;
    }
  }
  state.publicKeyOwners.set(
    wire.candidate_public_key_fingerprint,
    challenge.userId,
  );
  state.devices.set(`device:${completion.subject_handle}`, {
    userId: challenge.userId,
    active: true,
  });
  state.sessions.set(`session:${completion.subject_handle}`, {
    userId: challenge.userId,
    active: true,
  });
  state.refreshFamilies.set(`family:${completion.subject_handle}`, {
    userId: challenge.userId,
    active: true,
  });
  state.auditEvents.add(`enrollment-audit:${completion.subject_handle}`);
  state.notifications.add(
    `enrollment-notification:${completion.subject_handle}`,
  );
  state.outboxRecords.add(
    `enrollment-audit-outbox:${completion.subject_handle}`,
  );
  state.outboxRecords.add(
    `enrollment-notification-outbox:${completion.subject_handle}`,
  );
  return "CONSUMED_SUCCESS";
}

function challengeOutcome(item) {
  if (item.synthetic) {
    return "REJECT_SYNTHETIC_NO_STATE";
  }
  if (item.already_attempted) {
    return "REJECT_REPLAY";
  }
  if (isoMilliseconds(item.now) >= isoMilliseconds(item.expires_at)) {
    return "REJECT_EXPIRED";
  }
  return item.proof_valid
    ? "CONSUMED_SUCCESS"
    : "CONSUMED_INVALID_PROOF";
}

function refreshOutcome(item) {
  const now = isoMilliseconds(item.now);
  if (now >= isoMilliseconds(item.family_deadline)) {
    return "REJECT_FAMILY_EXPIRED";
  }
  if (item.status === "ROTATED") {
    return "REVOKE_FAMILY_AND_DESCENDANTS";
  }
  if (item.status === "REVOKED") {
    return "REJECT_REVOKED";
  }
  if (now >= isoMilliseconds(item.individual_expires_at)) {
    return "REJECT_EXPIRED_NO_ROTATION";
  }
  return "ROTATE_CREATE_DESCENDANT";
}

function presentRefreshToken(family, tokenDigest, now, descendantDigest) {
  const token = family.tokens.find(
    ({ token_digest: digest }) => digest === tokenDigest,
  );
  assert(token, "presented refresh digest is not in the locked family");
  const outcome = refreshOutcome({
    status: token.status,
    now,
    individual_expires_at: token.expires_at,
    family_deadline: family.family_deadline,
  });
  if (outcome === "ROTATE_CREATE_DESCENDANT") {
    assert(descendantDigest, "rotation requires a server-generated descendant");
    token.status = "ROTATED";
    token.rotated_to_digest = descendantDigest;
    const issuedAt = isoMilliseconds(now);
    family.tokens.push({
      schema_version: "fit.platform.refresh-record.v1",
      token_digest: descendantDigest,
      family_id: family.family_id,
      session_id: family.session_id,
      family_created_at: family.family_created_at,
      issued_at: now,
      expires_at: new Date(
        Math.min(
          issuedAt + 604_800_000,
          isoMilliseconds(family.family_deadline),
        ),
      )
        .toISOString()
        .replace(".000Z", "Z"),
      family_deadline: family.family_deadline,
      status: "ACTIVE",
    });
  } else if (outcome === "REVOKE_FAMILY_AND_DESCENDANTS") {
    family.status = "REVOKED";
    for (const member of family.tokens) {
      member.status = "REVOKED";
      member.revoked_at = now;
    }
  }
  return outcome;
}

function websocketOutcome(item) {
  if (item.revoked) {
    return "CLOSE_REVOKED_WITHIN_5_SECONDS";
  }
  if (item.event === "APPLICATION_FRAME_WHILE_REAUTH_PENDING") {
    return isoMilliseconds(item.frame_received_at) >=
      isoMilliseconds(item.current_access_expires_at)
      ? "REJECT_APPLICATION_FRAME"
      : "ALLOW_APPLICATION_FRAME";
  }
  if (item.event === "REAUTH_DEADLINE_WITHOUT_RESPONSE") {
    return "CLOSE_4401";
  }
  if (
    !item.challenge_issued ||
    item.malformed ||
    !item.nonce_matches ||
    item.nonce_consumed ||
    !item.user_matches ||
    !item.account_matches ||
    !item.device_matches ||
    !item.session_matches ||
    !item.family_matches ||
    !item.token_valid ||
    !item.token_fresh ||
    item.credential_source !== "HTTPS_REFRESH" ||
    isoMilliseconds(item.verification_completed_at) >=
      isoMilliseconds(item.deadline)
  ) {
    return "CLOSE_4401";
  }
  return "ATOMIC_REBIND";
}

function idempotencyOutcome(item) {
  if (!item.same_scope) {
    return "INDEPENDENT_SCOPE_NO_CROSS_OWNER_ACCESS";
  }
  if (item.existing_digest === null) {
    return "CLAIM_AND_EXECUTE";
  }
  return item.existing_digest === item.incoming_digest
    ? "RETURN_RECORDED_RESULT"
    : "IDEMPOTENCY_CONFLICT";
}

function strictRequestDecodeOutcome(item) {
  return item.duplicate_query_keys ||
    !item.json_mutation ||
    item.unknown_fields ||
    item.ambiguous_path
    ? "REJECT"
    : "ACCEPT";
}

function newThrottleDimension() {
  return {
    failureTimesMs: [],
    failures: 0,
    nextAllowedMs: 0,
    lockedUntilMs: 0,
  };
}

function normalizeThrottleState(state, nowMs, windowMs) {
  if (state.lockedUntilMs > 0 && nowMs >= state.lockedUntilMs) {
    state.failureTimesMs = [];
    state.failures = 0;
    state.nextAllowedMs = 0;
    state.lockedUntilMs = 0;
    return;
  }
  state.failureTimesMs = state.failureTimesMs.filter(
    (failureAtMs) => failureAtMs > nowMs - windowMs,
  );
  state.failures = state.failureTimesMs.length;
  if (state.failures === 0 && state.lockedUntilMs === 0) {
    state.nextAllowedMs = 0;
  }
}

function runThrottleTimeline(timeline, throttle) {
  const sources = new Map();
  const accounts = new Map();
  for (const event of timeline.events) {
    const nowMs = event.at_seconds * 1000;
    const sourceKey = event.source_key ?? timeline.source_key;
    const source = sources.get(sourceKey) ?? newThrottleDimension();
    sources.set(sourceKey, source);
    const account =
      event.user_id === null
        ? null
        : (accounts.get(event.user_id) ?? newThrottleDimension());
    if (account !== null) accounts.set(event.user_id, account);
    normalizeThrottleState(source, nowMs, throttle.window_seconds * 1000);
    if (account !== null) {
      normalizeThrottleState(account, nowMs, throttle.window_seconds * 1000);
    }
    const dimensions = [source, ...(account === null ? [] : [account])];
    let decision;
    let appliedDelaySeconds = 0;
    if (event.transaction_aborted === true) {
      decision = "TRANSACTION_ABORTED";
    } else if (dimensions.some((state) => nowMs < state.lockedUntilMs)) {
      decision = "DENY_LOCK";
    } else if (dimensions.some((state) => nowMs < state.nextAllowedMs)) {
      decision = "DENY_DELAY";
    } else if (event.password_valid) {
      assert(account !== null, `${timeline.name} accepted unresolved password`);
      account.failureTimesMs = [];
      account.failures = 0;
      account.nextAllowedMs = 0;
      account.lockedUntilMs = 0;
      decision = "PASSWORD_ACCEPTED";
    } else {
      for (const state of dimensions) {
        state.failureTimesMs.push(nowMs);
        state.failures = state.failureTimesMs.length;
        if (state.failures >= throttle.lock_on_failure) {
          state.lockedUntilMs = nowMs + throttle.lock_seconds * 1000;
          state.nextAllowedMs = 0;
        } else {
          appliedDelaySeconds = Math.max(
            appliedDelaySeconds,
            throttle.failure_delays_seconds[state.failures - 1],
          );
          state.nextAllowedMs = nowMs + appliedDelaySeconds * 1000;
        }
      }
      decision = dimensions.some((state) => state.lockedUntilMs > nowMs)
        ? "LOCKED"
        : "FAILED_DELAY";
    }
    assert.equal(decision, event.expected_decision, `${timeline.name} decision`);
    assert.equal(
      account?.failures ?? 0,
      event.expected_account_failures,
      `${timeline.name} account failures`,
    );
    assert.equal(
      source.failures,
      event.expected_source_failures,
      `${timeline.name} source failures`,
    );
    if (Object.hasOwn(event, "expected_delay_seconds")) {
      assert.equal(
        appliedDelaySeconds,
        event.expected_delay_seconds,
        `${timeline.name} delay`,
      );
    }
    if (Object.hasOwn(event, "expected_lock_until_seconds")) {
      assert.equal(
        source.lockedUntilMs / 1000,
        event.expected_lock_until_seconds,
        `${timeline.name} source lock expiry`,
      );
      if (account !== null) {
        assert.equal(
          account.lockedUntilMs / 1000,
          event.expected_lock_until_seconds,
          `${timeline.name} account lock expiry`,
        );
      }
    }
    if (Object.hasOwn(event, "expected_source_lock_until_seconds")) {
      assert.equal(
        source.lockedUntilMs / 1000,
        event.expected_source_lock_until_seconds,
        `${timeline.name} source-only lock expiry`,
      );
    }
    if (Object.hasOwn(event, "expected_account_lock_until_seconds")) {
      assert.equal(
        account?.lockedUntilMs / 1000,
        event.expected_account_lock_until_seconds,
        `${timeline.name} account-only lock expiry`,
      );
    }
  }
}

function tokenBucketDecisions(group, timestampsMs) {
  let tokens = group.burst;
  let lastMs = timestampsMs[0] ?? 0;
  return timestampsMs.map((nowMs) => {
    const elapsedMs = Math.max(0, nowMs - lastMs);
    tokens = Math.min(
      group.burst,
      tokens + (elapsedMs / 1000) * (group.rate / group.per_seconds),
    );
    lastMs = nowMs;
    if (tokens + Number.EPSILON < 1) return false;
    tokens -= 1;
    return true;
  });
}

function parseIpv4(value) {
  const parts = value.split(".");
  assert.equal(parts.length, 4, `invalid IPv4 address ${value}`);
  const octets = parts.map((part) => {
    assert(/^(?:0|[1-9][0-9]{0,2})$/.test(part), `invalid IPv4 ${value}`);
    const octet = Number(part);
    assert(octet <= 255, `invalid IPv4 ${value}`);
    return octet;
  });
  return Buffer.from(octets);
}

function canonicalIpBytes(value) {
  if (value.includes(".")) {
    const result = Buffer.alloc(16);
    result[10] = 0xff;
    result[11] = 0xff;
    parseIpv4(value).copy(result, 12);
    return result;
  }

  assert(
    /^[0-9a-fA-F:]+$/.test(value) && !value.includes(":::"),
    `invalid IPv6 address ${value}`,
  );
  const doubleColonParts = value.split("::");
  assert(doubleColonParts.length <= 2, `invalid IPv6 address ${value}`);
  const left =
    doubleColonParts[0] === "" ? [] : doubleColonParts[0].split(":");
  const right =
    doubleColonParts.length === 1 || doubleColonParts[1] === ""
      ? []
      : doubleColonParts[1].split(":");
  const omitted =
    doubleColonParts.length === 2 ? 8 - left.length - right.length : 0;
  assert(
    (doubleColonParts.length === 2 && omitted >= 1) ||
      (doubleColonParts.length === 1 && left.length === 8),
    `invalid IPv6 group count ${value}`,
  );
  const groups = [...left, ...Array(omitted).fill("0"), ...right];
  assert.equal(groups.length, 8, `invalid IPv6 group count ${value}`);
  const result = Buffer.alloc(16);
  groups.forEach((group, index) => {
    assert(/^[0-9a-fA-F]{1,4}$/.test(group), `invalid IPv6 group ${group}`);
    result.writeUInt16BE(Number.parseInt(group, 16), index * 2);
  });
  return result;
}

function walOutcome(item) {
  const unhealthy =
    item.archive_command_failed || item.archive_age_seconds > 60;
  if (unhealthy) {
    return item.incident_open_before
      ? "KEEP_SAME_INCIDENT_NO_DUPLICATE"
      : "OPEN_ONE_INCIDENT";
  }
  if (
    item.incident_open_before &&
    item.consecutive_healthy_cycles + 1 >= 2
  ) {
    return "CLEAR_AFTER_SECOND_SUCCESS";
  }
  return item.incident_open_before ? "KEEP_INCIDENT_OPEN" : "HEALTHY";
}

function assertDeepEqual(actual, expected, message) {
  assert.deepEqual(actual, expected, message);
}

const [
  platformSchema,
  remediationSchema,
  recoverySchema,
  validFixtureSet,
  invalidFixtureSet,
  scenarios,
  executableSecurity,
  linkageChains,
  golden,
  security,
  rateLimits,
  nats,
  transactions,
  migration,
  recoveryPolicy,
  failureInjection,
  stateMachines,
  phase0Manifest,
] = await Promise.all([
  readJson(path.join(platformDirectory, "schemas", "platform-v1.schema.json")),
  readJson(
    path.join(
      platformDirectory,
      "schemas",
      "remediation-proposal-v1.schema.json",
    ),
  ),
  readJson(
    path.join(
      platformDirectory,
      "schemas",
      "recovery-evidence-v1.schema.json",
    ),
  ),
  readJson(path.join(fixtureDirectory, "valid-schema-cases-v1.json")),
  readJson(path.join(fixtureDirectory, "invalid-schema-cases-v1.json")),
  readJson(path.join(fixtureDirectory, "semantic-scenarios-v1.json")),
  readJson(
    path.join(
      fixtureDirectory,
      "executable-security-scenarios-v1.json",
    ),
  ),
  readJson(path.join(fixtureDirectory, "linkage-chains-v1.json")),
  readJson(path.join(fixtureDirectory, "golden-vectors-v1.json")),
  readJson(
    path.join(platformDirectory, "manifests", "security-values-v1.json"),
  ),
  readJson(
    path.join(platformDirectory, "manifests", "http-rate-limits-v1.json"),
  ),
  readJson(
    path.join(platformDirectory, "manifests", "nats-permissions-v1.json"),
  ),
  readJson(
    path.join(
      platformDirectory,
      "manifests",
      "transaction-boundaries-v1.json",
    ),
  ),
  readJson(
    path.join(platformDirectory, "manifests", "migration-policy-v1.json"),
  ),
  readJson(
    path.join(platformDirectory, "manifests", "recovery-policy-v1.json"),
  ),
  readJson(
    path.join(platformDirectory, "manifests", "failure-injection-v1.json"),
  ),
  readJson(
    path.join(platformDirectory, "manifests", "state-machines-v1.json"),
  ),
  readJson(
    path.join(
      platformDirectory,
      "manifests",
      "phase0-file-manifest-v1.json",
    ),
  ),
]);

const ajv = new Ajv2020({
  allErrors: true,
  strict: true,
  strictRequired: false,
  strictTypes: false,
  validateFormats: true,
});
addFormats(ajv);
ajv.addKeyword({
  keyword: "x-fit-authority-field-names",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    const deniedFields = new Set(
      platformSchema.$defs.UntrustedJsonObject.propertyNames.not.enum.map(
        (field) => field.toLowerCase(),
      ),
    );
    return !containsCaseInsensitiveField(data, deniedFields);
  },
});
ajv.addKeyword({
  keyword: "x-fit-model-authority-fields",
  schemaType: "array",
  type: "object",
  errors: false,
  validate: (fields, data) =>
    !containsCaseInsensitiveField(
      data,
      new Set(fields.map((field) => field.toLowerCase())),
    ),
});
ajv.addKeyword({
  keyword: "x-fit-time-window",
  schemaType: "object",
  type: "object",
  errors: false,
  validate: (rule, data) => {
    try {
      const start = Date.parse(data[rule.start]);
      const end = Date.parse(data[rule.end]);
      return (
        Number.isFinite(start) &&
        Number.isFinite(end) &&
        end - start === rule.seconds * 1000 &&
        (!Object.hasOwn(data, "revoked_at") ||
          Date.parse(data.revoked_at) >= start)
      );
    } catch {
      return false;
    }
  },
});
ajv.addKeyword({
  keyword: "x-fit-time-order",
  schemaType: "array",
  type: "object",
  errors: false,
  validate: (rules, data) => {
    try {
      return rules.every(({ start, end }) => {
        if (!Object.hasOwn(data, end)) return true;
        const startMs = Date.parse(data[start]);
        const endMs = Date.parse(data[end]);
        return (
          Number.isFinite(startMs) &&
          Number.isFinite(endMs) &&
          endMs >= startMs
        );
      });
    } catch {
      return false;
    }
  },
});
ajv.addKeyword({
  keyword: "x-fit-refresh-window",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    try {
      const familyCreatedAt = Date.parse(data.family_created_at);
      const issuedAt = Date.parse(data.issued_at);
      const familyDeadline = Date.parse(data.family_deadline);
      const expiresAt = Date.parse(data.expires_at);
      const revokedAt = Object.hasOwn(data, "revoked_at")
        ? Date.parse(data.revoked_at)
        : null;
      return (
        Number.isFinite(familyCreatedAt) &&
        Number.isFinite(issuedAt) &&
        Number.isFinite(familyDeadline) &&
        Number.isFinite(expiresAt) &&
        issuedAt >= familyCreatedAt &&
        issuedAt < familyDeadline &&
        familyDeadline - familyCreatedAt === 2_592_000_000 &&
        expiresAt === Math.min(issuedAt + 604_800_000, familyDeadline) &&
        (!Object.hasOwn(data, "rotated_to_digest") ||
          data.rotated_to_digest !== data.token_digest) &&
        (revokedAt === null ||
          (Number.isFinite(revokedAt) &&
            revokedAt >= issuedAt &&
            revokedAt <= familyDeadline))
      );
    } catch {
      return false;
    }
  },
});
ajv.addKeyword({
  keyword: "x-fit-refresh-family-lineage",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    try {
      const tokens = data.tokens;
      const byDigest = new Map(tokens.map((token) => [token.token_digest, token]));
      if (byDigest.size !== tokens.length) return false;
      const referencedDigests = new Set();
      for (const token of tokens) {
        if (
          token.family_id !== data.family_id ||
          token.session_id !== data.session_id ||
          token.family_created_at !== data.family_created_at ||
          token.family_deadline !== data.family_deadline
        ) {
          return false;
        }
        if (Object.hasOwn(token, "rotated_to_digest")) {
          const descendant = byDigest.get(token.rotated_to_digest);
          if (
            descendant === undefined ||
            Date.parse(descendant.issued_at) < Date.parse(token.issued_at)
          ) {
            return false;
          }
          if (referencedDigests.has(token.rotated_to_digest)) return false;
          referencedDigests.add(token.rotated_to_digest);
        }
        if (data.status === "REVOKED" && token.status !== "REVOKED") return false;
      }
      const roots = tokens.filter(
        ({ token_digest: digest }) => !referencedDigests.has(digest),
      );
      if (
        roots.length !== 1 ||
        referencedDigests.size !== tokens.length - 1
      ) {
        return false;
      }
      if (
        data.status === "ACTIVE" &&
        (tokens.filter(({ status }) => status === "ACTIVE").length !== 1 ||
          tokens.some(({ status }) => status === "REVOKED"))
      ) {
        return false;
      }
      for (const origin of tokens) {
        const visited = new Set();
        let current = origin;
        while (current && Object.hasOwn(current, "rotated_to_digest")) {
          if (visited.has(current.token_digest)) return false;
          visited.add(current.token_digest);
          current = byDigest.get(current.rotated_to_digest);
        }
      }
      return true;
    } catch {
      return false;
    }
  },
});
ajv.addKeyword({
  keyword: "x-fit-websocket-deadline",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    try {
      const issuedAt = Date.parse(data.issued_at);
      const connectedAt = Date.parse(data.connection_established_at);
      const deadline = Date.parse(data.deadline);
      const accessExpiry = Date.parse(data.current_access_expires_at);
      const requiredIssueTime = Math.max(connectedAt, accessExpiry - 60_000);
      return (
        Number.isFinite(issuedAt) &&
        Number.isFinite(connectedAt) &&
        Number.isFinite(deadline) &&
        Number.isFinite(accessExpiry) &&
        connectedAt < accessExpiry &&
        issuedAt === requiredIssueTime &&
        issuedAt < accessExpiry &&
        deadline === Math.min(issuedAt + 60_000, accessExpiry)
      );
    } catch {
      return false;
    }
  },
});
ajv.addKeyword({
  keyword: "x-fit-payload-integrity",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    try {
      return (
        data !== null &&
        typeof data === "object" &&
        data.payload !== null &&
        typeof data.payload === "object" &&
        !Array.isArray(data.payload) &&
        data.payload.schema_version === "fit.platform.event-payload.v1" &&
        data.payload.schema_version === data.payload_schema_version &&
        data.payload.subject === data.subject &&
        data.payload.event_kind === data.event_kind &&
        jcsCanonicalize(data.payload.scope) === jcsCanonicalize(data.scope) &&
        sha256(jcsCanonicalize(data.payload)) === data.payload_digest &&
        data.payload.data.record.kind === data.event_kind &&
        jcsCanonicalize(data.payload.data.record.scope) ===
          jcsCanonicalize(data.scope) &&
        data.payload.data.record.causation_id === data.causation_id &&
        data.payload.data.record.correlation_id === data.correlation_id &&
        (data.payload.data.record_type === "audit"
          ? !data.subject.includes(".notification.") &&
            data.payload.data.record.event_id ===
              data.payload.data.record_id
          : data.payload.data.record_type === "notification" &&
            data.subject.includes(".notification.") &&
            data.payload.data.record.notification_id ===
              data.payload.data.record_id)
      );
    } catch {
      return false;
    }
  },
});
ajv.addKeyword({
  keyword: "x-fit-outbox-causality",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    try {
      const occurredAt = Date.parse(data.event.occurred_at);
      const createdAt = Date.parse(data.created_at);
      const publishedAt = Object.hasOwn(data, "published_at")
        ? Date.parse(data.published_at)
        : null;
      return (
        Number.isFinite(occurredAt) &&
        Number.isFinite(createdAt) &&
        occurredAt <= createdAt &&
        (publishedAt === null ||
          (Number.isFinite(publishedAt) && createdAt <= publishedAt))
      );
    } catch {
      return false;
    }
  },
});
ajv.addKeyword({
  keyword: "x-fit-recovery-consistency",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    try {
      const committedAt = Date.parse(data.last_committed.committed_at);
      const recoveredAt = Date.parse(data.last_recovered.committed_at);
      const injectedAt = Date.parse(data.failure.injected_at);
      const invokedAt = Date.parse(data.restore.invoked_at);
      const terminalAt = Date.parse(
        data.result === "PASS"
          ? data.restore.verification_completed_at
          : data.restore.verification_stopped_at,
      );
      const terminalEventId =
        data.result === "PASS"
          ? data.restore.completion_event_id
          : data.restore.stop_event_id;
      const failClosedKinds = new Set([
        "BACKUP_CORRUPTION",
        "MISSING_WAL",
        "INCOMPATIBLE_MIGRATION",
        "PARTIAL_RESTORE",
        "MISSING_CREDENTIAL",
        "EXTERNAL_EGRESS",
      ]);
      const gateStatuses = data.verification_gates.map(({ status }) => status);
      const componentNames = data.components.map(({ name }) => name).sort();
      return (
        [
          committedAt,
          recoveredAt,
          injectedAt,
          invokedAt,
          terminalAt,
        ].every(Number.isFinite) &&
        data.last_committed.sequence === 6000 &&
        data.last_recovered.sequence <= data.last_committed.sequence &&
        recoveredAt <= committedAt &&
        committedAt === injectedAt &&
        injectedAt <= invokedAt &&
        invokedAt <= terminalAt &&
        data.restore.start_event_id !== terminalEventId &&
        data.observed_rpo.sequence_gap ===
          data.last_committed.sequence - data.last_recovered.sequence &&
        data.observed_rpo.time_gap_ms === committedAt - recoveredAt &&
        (data.result === "PASS"
          ? data.observed_rto_ms === terminalAt - invokedAt
          : !Object.hasOwn(data, "observed_rto_ms")) &&
        jcsCanonicalize(componentNames) ===
          jcsCanonicalize(["postgresql", "recovery-tool"]) &&
        (!failClosedKinds.has(data.failure.kind) ||
          data.result === "FAIL_CLOSED") &&
        (data.result === "PASS"
          ? gateStatuses.every((status) => status === "PASS")
          : gateStatuses.some((status) => status === "FAIL") &&
            typeof data.restore.failure_reason_code === "string")
      );
    } catch {
      return false;
    }
  },
});
ajv.addSchema(platformSchema);

const platformValidators = new Map();
for (const definitionName of Object.keys(platformSchema.$defs)) {
  platformValidators.set(
    definitionName,
    ajv.compile({
      $ref: `${platformSchema.$id}#/$defs/${definitionName}`,
    }),
  );
}
const remediationValidator = ajv.compile(remediationSchema);
const recoveryValidator = ajv.compile(recoverySchema);

function validatorFor(schemaName) {
  if (schemaName === "RemediationProposal") {
    return remediationValidator;
  }
  if (schemaName === "RecoveryEvidence") {
    return recoveryValidator;
  }
  const validator = platformValidators.get(schemaName);
  assert(validator, `fixture references unknown schema ${schemaName}`);
  return validator;
}

for (const fixture of validFixtureSet.cases) {
  const validate = validatorFor(fixture.schema);
  assert(
    validate(fixture.value),
    `valid fixture failed: ${fixture.name}\n${ajv.errorsText(validate.errors, {
      separator: "\n",
    })}`,
  );
}

for (const fixture of invalidFixtureSet.cases) {
  const validate = validatorFor(fixture.schema);
  assert(
    !validate(fixture.value),
    `negative fixture unexpectedly passed: ${fixture.name}`,
  );
  assert(
    typeof fixture.reason === "string" && fixture.reason.length > 0,
    `negative fixture lacks reason: ${fixture.name}`,
  );
}

const validateUntrustedMutation = validatorFor("UntrustedMutationInput");
const validUntrustedMutation = validFixtureSet.cases.find(
  ({ schema }) => schema === "UntrustedMutationInput",
).value;
const recursiveAuthorityDenylist =
  platformSchema.$defs.UntrustedJsonObject.propertyNames.not.enum;
for (const serverOwnedField of transactions.server_owned_fields) {
  assert(
    recursiveAuthorityDenylist.includes(serverOwnedField),
    `server-owned field ${serverOwnedField} missing recursive denylist`,
  );
}
assertDeepEqual(
  [...security.server_authored_identity.fields].sort(),
  [...transactions.server_owned_fields].sort(),
  "security and transaction manifests disagree on server-owned fields",
);
for (const forbiddenField of recursiveAuthorityDenylist) {
  for (const spelling of [forbiddenField, forbiddenField.toUpperCase()]) {
    const mutation = clone(validUntrustedMutation);
    mutation.body = {
      nested: [{ deeper: { [spelling]: "client-forged-authority" } }],
    };
    assert(
      !validateUntrustedMutation(mutation),
      `untrusted recursive input accepted server authority ${spelling}`,
    );
  }
}
assertDeepEqual(transactions.model_tool_forbidden_authority_fields, [
  "confirmation_id",
  "confirmation_hash",
]);
assert.equal(
  security.server_authored_identity.authenticated_http_input_schema,
  "UntrustedMutationInput",
);
assert.equal(
  security.server_authored_identity.model_tool_input_schema,
  "ModelToolMutationInput",
);
const legitimateConfirmationInput = {
  ...clone(validUntrustedMutation),
  path: {
    confirmation_id: "70000000-0000-4000-8000-000000000001",
  },
  body: {
    confirmation_hash: "a".repeat(64),
  },
};
assert(
  validateUntrustedMutation(legitimateConfirmationInput),
  "HTTP confirmation input was incorrectly treated as server-authored identity",
);
const validateModelToolMutation = validatorFor("ModelToolMutationInput");
assert(
  !validateModelToolMutation(legitimateConfirmationInput),
  "model/tool input accepted confirmation authority",
);
const uppercaseModelAuthority = clone(legitimateConfirmationInput);
uppercaseModelAuthority.path = {
  CONFIRMATION_ID: "70000000-0000-4000-8000-000000000001",
};
delete uppercaseModelAuthority.body.confirmation_hash;
assert(
  !validateModelToolMutation(uppercaseModelAuthority),
  "model/tool input accepted uppercase confirmation authority",
);

for (const fixture of validFixtureSet.cases.filter(
  ({ schema }) =>
    schema === "EnrollmentChallenge" || schema === "DeviceActionChallenge",
)) {
  assert.equal(
    isoMilliseconds(fixture.value.expires_at) -
      isoMilliseconds(fixture.value.issued_at),
    security.challenge[
      fixture.schema === "EnrollmentChallenge"
        ? "enrollment"
        : "device_action"
    ].ttl_seconds * 1000,
    `${fixture.name} does not use the frozen Challenge TTL`,
  );
}
const validSession = validFixtureSet.cases.find(
  ({ schema }) => schema === "Session",
).value;
assert.equal(
  isoMilliseconds(validSession.access_expires_at) -
    isoMilliseconds(validSession.created_at),
  security.tokens.access_ttl_seconds * 1000,
  "access token TTL fixture drifted",
);
const validRefreshRecord = validFixtureSet.cases.find(
  ({ schema }) => schema === "RefreshTokenRecord",
).value;
assert.equal(
  isoMilliseconds(validRefreshRecord.expires_at),
  Math.min(
    isoMilliseconds(validRefreshRecord.issued_at) +
      security.tokens.refresh_individual_ttl_seconds * 1000,
    isoMilliseconds(validRefreshRecord.family_deadline),
  ),
  "refresh record does not use the earlier individual or family deadline",
);
assert.equal(
  isoMilliseconds(validRefreshRecord.family_deadline) -
    isoMilliseconds(validRefreshRecord.family_created_at),
  security.tokens.refresh_family_max_lifetime_seconds * 1000,
  "refresh family maximum lifetime drifted",
);
const validRefreshFamily = validFixtureSet.cases.find(
  ({ schema }) => schema === "RefreshFamilyState",
).value;
const validateRefreshFamily = validatorFor("RefreshFamilyState");
for (const mutate of [
  (value) => {
    value.tokens[1].family_id =
      "50000000-0000-4000-8000-000000000099";
  },
  (value) => {
    value.tokens[1].status = "ROTATED";
    value.tokens[1].rotated_to_digest = value.tokens[0].token_digest;
  },
  (value) => {
    value.status = "REVOKED";
  },
]) {
  const invalidFamily = clone(validRefreshFamily);
  mutate(invalidFamily);
  assert(
    !validateRefreshFamily(invalidFamily),
    "refresh family accepted cross-family, cyclic, or partially revoked lineage",
  );
}
const executableRefreshFamily = {
  schema_version: "fit.platform.refresh-family-state.v1",
  family_id: validRefreshRecord.family_id,
  session_id: validRefreshRecord.session_id,
  family_created_at: validRefreshRecord.family_created_at,
  family_deadline: validRefreshRecord.family_deadline,
  status: "ACTIVE",
  tokens: [clone(validRefreshRecord)],
};
assert.equal(
  presentRefreshToken(
    executableRefreshFamily,
    validRefreshRecord.token_digest,
    "2026-08-01T10:00:00Z",
    "b".repeat(64),
  ),
  "ROTATE_CREATE_DESCENDANT",
);
assert(validateRefreshFamily(executableRefreshFamily));
assert.equal(
  presentRefreshToken(
    executableRefreshFamily,
    validRefreshRecord.token_digest,
    "2026-08-02T10:00:00Z",
  ),
  "REVOKE_FAMILY_AND_DESCENDANTS",
);
assert(validateRefreshFamily(executableRefreshFamily));
assert(
  executableRefreshFamily.tokens.every(({ status }) => status === "REVOKED"),
  "refresh reuse did not revoke every already-issued descendant",
);
for (const mutation of [
  { ...validSession, access_expires_at: "2026-07-29T11:00:00Z" },
  { ...validSession, revoked_at: "2026-07-29T10:01:00Z" },
  {
    ...validSession,
    status: "REVOKED",
    revoked_at: "2026-07-29T09:59:59Z",
  },
]) {
  assert(
    !validatorFor("Session")(mutation),
    "Session accepted contradictory state or non-frozen access lifetime",
  );
}
for (const mutation of [
  { ...validRefreshRecord, expires_at: "2026-08-30T10:00:00Z" },
  {
    ...validRefreshRecord,
    family_deadline: "2026-10-29T10:00:00Z",
  },
  {
    ...validRefreshRecord,
    family_created_at: "2026-07-29T10:00:01Z",
  },
  {
    ...validRefreshRecord,
    status: "ROTATED",
    rotated_to_digest: validRefreshRecord.token_digest,
  },
  { ...validRefreshRecord, revoked_at: "2026-07-29T10:01:00Z" },
  {
    ...validRefreshRecord,
    status: "REVOKED",
    revoked_at: "2026-07-29T09:59:59Z",
  },
]) {
  assert(
    !validatorFor("RefreshTokenRecord")(mutation),
    "refresh record accepted contradictory state or invalid expiry formula",
  );
}
const validOwnership = validFixtureSet.cases.find(
  ({ schema }) => schema === "TradingAccountOwnership",
).value;
const validDevice = validFixtureSet.cases.find(
  ({ schema }) => schema === "Device",
).value;
const validOutboxRecord = validFixtureSet.cases.find(
  ({ schema }) => schema === "OutboxRecord",
).value;
for (const [schema, mutation] of [
  [
    "TradingAccountOwnership",
    {
      ...validOwnership,
      status: "REVOKED",
      revoked_at: "2026-07-29T09:59:59Z",
    },
  ],
  [
    "Device",
    {
      ...validDevice,
      status: "REVOKED",
      revoked_at: "2026-07-29T09:59:59Z",
    },
  ],
  [
    "OutboxRecord",
    {
      ...validOutboxRecord,
      published_at: "2026-07-29T09:59:59Z",
    },
  ],
]) {
  assert(
    !validatorFor(schema)(mutation),
    `${schema} accepted lifecycle time before creation`,
  );
}
const validEnrollmentChallenge = validFixtureSet.cases.find(
  ({ schema }) => schema === "EnrollmentChallenge",
).value;
assert(
  !validatorFor("EnrollmentChallenge")({
    ...validEnrollmentChallenge,
    expires_at: "2026-07-29T11:00:00Z",
  }),
  "EnrollmentChallenge accepted a non-frozen lifetime",
);
assert(
  !validatorFor("EnrollmentChallenge")({
    ...validEnrollmentChallenge,
    issued_at: "2026-07-29T10:00:00.0001Z",
    expires_at: "2026-07-29T10:02:00.0001Z",
  }),
  "EnrollmentChallenge accepted timestamp precision finer than milliseconds",
);
const validWebSocketChallenge = validFixtureSet.cases.find(
  ({ schema }) => schema === "WebSocketReauthRequired",
).value;
assert(
  !validatorFor("WebSocketReauthRequired")({
    ...validWebSocketChallenge,
    deadline: "2026-07-29T10:30:00Z",
  }),
  "WebSocket challenge accepted deadline after access expiry",
);

const validRemediation = validFixtureSet.cases.find(
  (fixture) => fixture.schema === "RemediationProposal",
).value;
for (const requiredField of remediationSchema.required) {
  const mutation = clone(validRemediation);
  delete mutation[requiredField];
  assert(
    !remediationValidator(mutation),
    `remediation must reject missing ${requiredField}`,
  );
}

const notificationManifest = security.notification.kind_severity;
const notificationDetails = {
  LOGIN_ACCOUNT_LOCKED: {
    code: "security.login.account_locked",
    args: { lock_seconds: 900 },
    scope: "AUTH_SECURITY",
  },
  LOGIN_SOURCE_LOCKED: {
    code: "security.login.source_locked",
    args: { lock_seconds: 900 },
    scope: "AUTH_SECURITY",
  },
  DEVICE_ENROLLED: {
    code: "security.device.enrolled",
    args: { device_id: "30000000-0000-4000-8000-000000000001" },
    scope: "AUTH_SECURITY",
  },
  DEVICE_REPLACED: {
    code: "security.device.replaced",
    args: { device_id: "30000000-0000-4000-8000-000000000001" },
    scope: "AUTH_SECURITY",
  },
  DEVICE_REVOKED: {
    code: "security.device.revoked",
    args: { device_id: "30000000-0000-4000-8000-000000000001" },
    scope: "AUTH_SECURITY",
  },
  SESSION_REVOKED: {
    code: "security.session.revoked",
    args: { session_id: "40000000-0000-4000-8000-000000000001" },
    scope: "AUTH_SECURITY",
  },
  REFRESH_REUSE_DETECTED: {
    code: "security.refresh.reuse_detected",
    args: { session_id: "40000000-0000-4000-8000-000000000001" },
    scope: "AUTH_SECURITY",
  },
  WAL_ARCHIVE_INTERRUPTED: {
    code: "system.wal.archive_interrupted",
    args: {
      incident_id: "f0000000-0000-4000-8000-000000000002",
      archive_age_seconds: 61,
    },
    scope: "SYSTEM",
  },
};
assertDeepEqual(
  Object.keys(notificationDetails).sort(),
  Object.keys(notificationManifest).sort(),
  "notification fixture generator must cover every frozen kind",
);
const validateNotificationIntent = validatorFor("NotificationIntent");
for (const [kind, severity] of Object.entries(notificationManifest)) {
  const details = notificationDetails[kind];
  const scope =
    details.scope === "SYSTEM"
      ? { type: "SYSTEM" }
      : {
          type: "AUTH_SECURITY",
          source_key:
            "src_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
          ...(kind === "LOGIN_ACCOUNT_LOCKED"
            ? { user_id: "10000000-0000-4000-8000-000000000001" }
            : {}),
        };
  const value = {
    schema_version: "fit.platform.notification-intent.v1",
    kind,
    severity,
    scope,
    message_code: details.code,
    message_args: details.args,
    causation_id: "80000000-0000-4000-8000-000000000001",
    correlation_id: "b0000000-0000-4000-8000-000000000001",
  };
  assert(
    validateNotificationIntent(value),
    `generated valid notification failed for ${kind}: ${ajv.errorsText(
      validateNotificationIntent.errors,
    )}`,
  );
  for (const wrongSeverity of ["INFO", "WARNING", "CRITICAL"].filter(
    (candidate) => candidate !== severity,
  )) {
    const mutation = { ...value, severity: wrongSeverity };
    assert(
      !validateNotificationIntent(mutation),
      `${kind} must reject severity ${wrongSeverity}`,
    );
  }
  for (const forbiddenField of security.notification.forbidden_fields) {
    const mutation = { ...value, [forbiddenField]: "synthetic-forbidden-value" };
    assert(
      !validateNotificationIntent(mutation),
      `${kind} must reject notification field ${forbiddenField}`,
    );
  }
}

for (const scenario of scenarios.challenge_cases) {
  assert.equal(
    challengeOutcome(scenario),
    scenario.expected,
    `challenge case failed: ${scenario.name}`,
  );
}
const syntheticHandles = new Set();
const enrollmentCompletionFor = (wire, overrides = {}) => ({
  subject_handle: wire.subject_handle,
  purpose: wire.purpose,
  nonce: wire.nonce,
  candidate_public_key_fingerprint: wire.candidate_public_key_fingerprint,
  candidate_public_key: golden.ed25519_proofs.public_key,
  signature: golden.ed25519_proofs.vectors.find(
    ({ challenge_schema }) => challenge_schema === "EnrollmentChallenge",
  ).signature,
  completed_at: "2026-07-29T10:01:00Z",
  ...overrides,
});
function signedSyntheticEnrollment(baseWire, overrides = {}) {
  const { publicKey, privateKey } = generateKeyPairSync("ed25519");
  const publicDer = publicKey.export({ format: "der", type: "spki" });
  const rawPublicKey = publicDer.subarray(publicDer.length - 32);
  const wire = {
    ...clone(baseWire),
    ...overrides,
    candidate_public_key_fingerprint: `ed25519:${sha256(rawPublicKey)}`,
  };
  const canonical = jcsCanonicalize(wire);
  const signedBytes = Buffer.concat([
    Buffer.from(wire.domain, "utf8"),
    Buffer.from([0]),
    Buffer.from(canonical, "utf8"),
  ]);
  return {
    wire,
    completion: {
      subject_handle: wire.subject_handle,
      purpose: wire.purpose,
      nonce: wire.nonce,
      candidate_public_key_fingerprint:
        wire.candidate_public_key_fingerprint,
      candidate_public_key: `ed25519-public:${rawPublicKey.toString("hex")}`,
      signature: `ed25519-signature:${signMessage(
        null,
        signedBytes,
        privateKey,
      ).toString("hex")}`,
      completed_at: "2026-07-29T10:01:00Z",
    },
  };
}
for (const scenario of executableSecurity.enrollment_transition_cases) {
  assert(
    validatorFor("EnrollmentChallenge")(scenario.wire),
    `synthetic challenge wire is invalid: ${scenario.name}`,
  );
  assert.match(scenario.input.password_attempt_sha256, /^[a-f0-9]{64}$/u);
  assert.equal(scenario.wire.purpose, scenario.input.purpose, scenario.name);
  const state = enrollmentStateFromFixture(executableSecurity);
  const startResult = startEnrollment(state, scenario.input, scenario.wire);
  assert.equal(
    startResult.persisted,
    scenario.expected_start_persisted,
    scenario.name,
  );
  assert.equal(
    completeEnrollment(
      state,
      enrollmentCompletionFor(
        startResult.wire,
        scenario.completion.proof_valid
          ? {}
          : { signature: `ed25519-signature:${"0".repeat(128)}` },
      ),
    ),
    scenario.expected_completion,
    scenario.name,
  );
  assertDeepEqual(
    {
      challenges: state.challenges.size,
      devices: state.devices.size,
      sessions: state.sessions.size,
      refresh_families: state.refreshFamilies.size,
      audit_events: state.auditEvents.size,
      notifications: state.notifications.size,
      outbox_records: state.outboxRecords.size,
    },
    scenario.expected_durable_counts,
    `${scenario.name} durable state delta`,
  );
  assert(!syntheticHandles.has(scenario.wire.subject_handle), scenario.name);
  syntheticHandles.add(scenario.wire.subject_handle);
  for (const forbidden of security.challenge.enrollment.wire_forbidden_fields) {
    assert(
      !Object.hasOwn(scenario.wire, forbidden),
      `${scenario.name} leaked ${forbidden}`,
    );
  }
}
const realEnrollmentScenario =
  executableSecurity.enrollment_transition_cases.find(
    ({ expected_completion: expected }) => expected === "CONSUMED_SUCCESS",
  );
{
  const state = enrollmentStateFromFixture(executableSecurity);
  startEnrollment(state, realEnrollmentScenario.input, realEnrollmentScenario.wire);
  const invalidBinding = enrollmentCompletionFor(
    realEnrollmentScenario.wire,
    { nonce: "synthetic_wrong_bound_nonce_000000000001" },
  );
  assert.equal(
    completeEnrollment(state, invalidBinding),
    "CONSUMED_INVALID_PROOF",
  );
  assert.equal(state.devices.size, 0);
  assert.equal(state.auditEvents.size, 2);
  assert.equal(state.outboxRecords.size, 2);
}
{
  const state = enrollmentStateFromFixture(executableSecurity);
  startEnrollment(state, realEnrollmentScenario.input, realEnrollmentScenario.wire);
  const completion = enrollmentCompletionFor(realEnrollmentScenario.wire);
  assert.equal(completeEnrollment(state, completion), "CONSUMED_SUCCESS");
  assert.equal(completeEnrollment(state, completion), "REJECT_REPLAY");
  assert.equal(state.devices.size, 1);
  assert.equal(state.auditEvents.size, 3);
  assert.equal(state.outboxRecords.size, 4);
}
for (const [existingOwner, expected] of [
  ["10000000-0000-4000-8000-000000000001", "REJECT_DUPLICATE_KEY"],
  ["10000000-0000-4000-8000-000000000099", "REJECT_CROSS_OWNER_REBIND"],
]) {
  const state = enrollmentStateFromFixture(executableSecurity);
  state.publicKeyOwners.set(
    realEnrollmentScenario.wire.candidate_public_key_fingerprint,
    existingOwner,
  );
  startEnrollment(state, realEnrollmentScenario.input, realEnrollmentScenario.wire);
  assert.equal(
    completeEnrollment(
      state,
      enrollmentCompletionFor(realEnrollmentScenario.wire),
    ),
    expected,
  );
  assert.equal(state.devices.size, 0);
  assert.equal(state.auditEvents.size, 2);
  assert.equal(state.outboxRecords.size, 2);
}
for (const purpose of ["ADDITIONAL_DEVICE", "REPLACEMENT_DEVICE"]) {
  const state = enrollmentStateFromFixture(executableSecurity);
  state.devices.set("prior-device", {
    userId: "10000000-0000-4000-8000-000000000001",
    active: true,
  });
  state.sessions.set("prior-session", {
    userId: "10000000-0000-4000-8000-000000000001",
    active: true,
  });
  state.refreshFamilies.set("prior-family", {
    userId: "10000000-0000-4000-8000-000000000001",
    active: true,
  });
  const { wire, completion } = signedSyntheticEnrollment(
    realEnrollmentScenario.wire,
    {
    purpose,
    subject_handle: `real_${purpose.toLowerCase()}_state_transition`,
    },
  );
  const input = { ...realEnrollmentScenario.input, purpose };
  startEnrollment(state, input, wire);
  assert.equal(
    completeEnrollment(state, completion),
    "CONSUMED_SUCCESS",
  );
  const priorShouldRemainActive = purpose === "ADDITIONAL_DEVICE";
  assert.equal(state.devices.get("prior-device").active, priorShouldRemainActive);
  assert.equal(state.sessions.get("prior-session").active, priorShouldRemainActive);
  assert.equal(
    state.refreshFamilies.get("prior-family").active,
    priorShouldRemainActive,
  );
}

for (const scenario of scenarios.refresh_cases) {
  assert.equal(
    refreshOutcome(scenario),
    scenario.expected,
    `refresh case failed: ${scenario.name}`,
  );
}

for (const scenario of scenarios.refresh_expiry_cases) {
  const issued = isoMilliseconds(scenario.issued_at);
  const deadline = isoMilliseconds(scenario.family_deadline);
  const calculated = Math.min(issued + 604_800_000, deadline);
  assert.equal(
    calculated,
    isoMilliseconds(scenario.expected_expires_at),
    `refresh expiry case failed: ${scenario.name}`,
  );
}

for (const scenario of scenarios.throttle_cases) {
  const delay =
    scenario.failure_number < 5
      ? security.password_throttle.failure_delays_seconds[
          scenario.failure_number - 1
        ]
      : 0;
  const lock =
    scenario.failure_number === security.password_throttle.lock_on_failure
      ? security.password_throttle.lock_seconds
      : 0;
  assert.equal(
    delay,
    scenario.expected_delay_seconds,
    `throttle delay failed: ${scenario.name}`,
  );
  assert.equal(
    lock,
    scenario.expected_lock_seconds,
    `throttle lock failed: ${scenario.name}`,
  );
}
for (const timeline of executableSecurity.password_throttle_timelines) {
  runThrottleTimeline(timeline, security.password_throttle);
}
const crossRouteTimeline = executableSecurity.password_throttle_timelines.find(
  ({ name }) => name.includes("shared cross-route"),
);
assertDeepEqual(
  [...new Set(crossRouteTimeline.events.map(({ route }) => route))].sort(),
  [
    "POST /v1/auth/device-enrollments/start",
    "POST /v1/auth/device-replacements/start",
    "POST /v1/auth/login",
  ],
  "shared throttle timeline does not cross all password routes",
);

const crossRoute = scenarios.cross_route_throttle_case;
let accountFailures = 0;
let sourceFailures = 0;
for (const attempt of crossRoute.attempts) {
  assert(
    security.password_throttle.routes.includes(attempt.route),
    `cross-route case used an unfrozen route ${attempt.route}`,
  );
  if (!attempt.success) {
    sourceFailures += 1;
    if (attempt.resolved_user) {
      accountFailures += 1;
    }
  }
}
assert.equal(accountFailures, crossRoute.expected_account_failures);
assert.equal(sourceFailures, crossRoute.expected_source_failures);
assert.equal(
  accountFailures >= 5 && sourceFailures >= 5,
  crossRoute.expected_both_locked,
);
for (const route of security.password_throttle.routes) {
  const cases = scenarios.password_failure_response_cases.filter(
    (scenario) => scenario.route === route,
  );
  assert.equal(cases.length, 2, `${route} needs unknown and wrong-password cases`);
  assertDeepEqual(
    cases.map(
      ({ status, envelope, timing_class, synthetic_challenge }) => ({
        status,
        envelope,
        timing_class,
        synthetic_challenge,
      }),
    ),
    [
      {
        status: cases[0].status,
        envelope: cases[0].envelope,
        timing_class: cases[0].timing_class,
        synthetic_challenge: cases[0].synthetic_challenge,
      },
      {
        status: cases[0].status,
        envelope: cases[0].envelope,
        timing_class: cases[0].timing_class,
        synthetic_challenge: cases[0].synthetic_challenge,
      },
    ],
    `${route} leaks unknown-identifier versus wrong-password state`,
  );
}

for (const scenario of scenarios.throttle_reset_cases) {
  let account = scenario.account_failures_before;
  let source = scenario.source_failures_before;
  if (scenario.event === "PASSWORD_SUCCESS") {
    account = 0;
  } else if (scenario.event === "PASSWORD_FAILURE_UNKNOWN_IDENTIFIER") {
    source += 1;
  } else if (scenario.event === "ACCOUNT_LOCK_EXPIRED") {
    account = 0;
  } else {
    assert.fail(`unknown throttle reset event ${scenario.event}`);
  }
  assert.equal(account, scenario.expected_account_failures, scenario.name);
  assert.equal(source, scenario.expected_source_failures, scenario.name);
}

for (const scenario of scenarios.websocket_cases) {
  assert.equal(
    websocketOutcome(scenario),
    scenario.expected,
    `WebSocket case failed: ${scenario.name}`,
  );
}
const validWebSocketReauth = scenarios.websocket_cases.find(
  ({ expected }) => expected === "ATOMIC_REBIND",
);
const validWebSocketChallengeForTiming = validFixtureSet.cases.find(
  ({ schema }) => schema === "WebSocketReauthRequired",
).value;
const prematurelyIssuedChallenge = clone(validWebSocketChallengeForTiming);
prematurelyIssuedChallenge.issued_at = "2026-07-29T10:01:00Z";
prematurelyIssuedChallenge.deadline = "2026-07-29T10:02:00Z";
assert(
  !validatorFor("WebSocketReauthRequired")(prematurelyIssuedChallenge),
  "WebSocket challenge was accepted before the exact reauthorization window",
);
for (const mismatchField of [
  "nonce_matches",
  "user_matches",
  "account_matches",
  "device_matches",
  "session_matches",
  "family_matches",
  "token_valid",
  "token_fresh",
]) {
  assert.equal(
    websocketOutcome({ ...validWebSocketReauth, [mismatchField]: false }),
    "CLOSE_4401",
    `WebSocket reauthorization accepted ${mismatchField}=false`,
  );
}
for (const scenario of scenarios.websocket_deadline_cases) {
  const requiredIssueTime = Math.max(
    isoMilliseconds(scenario.connection_established_at),
    isoMilliseconds(scenario.current_access_expires_at) - 60_000,
  );
  assert.equal(
    isoMilliseconds(scenario.issued_at),
    requiredIssueTime,
    `WebSocket issuance case failed: ${scenario.name}`,
  );
  const calculated = Math.min(
    isoMilliseconds(scenario.issued_at) + 60_000,
    isoMilliseconds(scenario.current_access_expires_at),
  );
  assert.equal(
    calculated,
    isoMilliseconds(scenario.expected_deadline),
    `WebSocket deadline case failed: ${scenario.name}`,
  );
}
const bindingFixture = validFixtureSet.cases.find(
  ({ schema }) => schema === "WebSocketBinding",
).value;
for (const scenario of executableSecurity.websocket_upgrade_cases) {
  const tokenClaims = {
    user_id: bindingFixture.user_id,
    trading_account_id: bindingFixture.trading_account_id,
    device_id: bindingFixture.device_id,
    session_id: bindingFixture.session_id,
    refresh_family_id: bindingFixture.refresh_family_id,
  };
  if (scenario.mismatch_field !== null) {
    tokenClaims[scenario.mismatch_field] =
      "99000000-0000-4000-8000-000000000099";
  }
  const matches = Object.entries(tokenClaims).every(
    ([field, value]) => bindingFixture[field] === value,
  );
  const accepted =
    matches &&
    scenario.token_signature_valid &&
    scenario.token_not_expired &&
    (scenario.user_active ?? true) &&
    (scenario.account_ownership_active ?? true) &&
    scenario.session_active &&
    scenario.device_active &&
    scenario.refresh_family_active;
  assert.equal(accepted ? "ACCEPT" : "REJECT", scenario.expected, scenario.name);
}
for (const scenario of executableSecurity.websocket_redaction_cases) {
  assert(
    security.websocket_reauthorization.redacted_fields.includes(
      scenario.field,
    ),
    `missing WebSocket redaction for ${scenario.field}`,
  );
  assertDeepEqual(
    [...scenario.forbidden_sinks].sort(),
    [...security.websocket_reauthorization.redaction_sinks].sort(),
    `${scenario.field} redaction sinks drifted`,
  );
}
assert.equal(
  security.websocket_reauthorization.upgrade_binding_validation,
  "VALID_UNEXPIRED_ACCESS_TOKEN_AND_ACTIVE_USER_ACCOUNT_DEVICE_SESSION_REFRESH_FAMILY_EXACT_MATCH",
);
assert.equal(
  security.websocket_reauthorization.application_frame_while_reauth_pending,
  "ALLOW_UNTIL_CURRENT_ACCESS_EXPIRY",
);
assert.equal(
  stateMachines.machines.websocket_reauthorization
    .pending_application_frame_before_expiry,
  "ALLOW",
);
assert.equal(
  security.websocket_reauthorization
    .sensitive_control_data_in_logs_traces_metrics_application_messages,
  false,
);

for (const scenario of scenarios.idempotency_cases) {
  assert.equal(
    idempotencyOutcome(scenario),
    scenario.expected,
    `idempotency case failed: ${scenario.name}`,
  );
}

for (const scenario of scenarios.request_decode_cases) {
  assert.equal(
    strictRequestDecodeOutcome(scenario),
    scenario.expected,
    `request decoding case failed: ${scenario.name}`,
  );
}
for (const [raw, accepted] of [
  ['{"body":{"decision":"CONFIRM"},"path":{}}', true],
  ['{"body":{"decision":"CONFIRM","decision":"REJECT"},"path":{}}', false],
  ['{"body":{"nested":{"scope":"A","scope":"B"}},"path":{}}', false],
  ['{"items":[{"nonce":"A"},{"nonce":"B"}]}', true],
  [' {"body":{}}', false],
  ['{"body":{"__proto__":{"authority":"client"}}}', false],
  ['{"body":{"constructor":{"authority":"client"}}}', false],
]) {
  let parsed = false;
  try {
    parseStrictJson(raw);
    parsed = true;
  } catch {
    parsed = false;
  }
  assert.equal(parsed, accepted, `strict raw JSON decision drifted for ${raw}`);
}
assertDeepEqual(security.request_digest.strict_raw_json_decoder.entry_points, {
  HTTP_MUTATION_BODY: "BEFORE_HANDLER_AND_SCHEMA_VALIDATION",
  WEBSOCKET_CONTROL_FRAME:
    "BEFORE_CONTROL_FRAME_DISPATCH_AND_SCHEMA_VALIDATION",
});
assertDeepEqual(
  security.request_digest.strict_raw_json_decoder.allowed_whitespace_bytes,
  ["0x20", "0x09", "0x0a", "0x0d"],
);
assertDeepEqual(
  security.request_digest.strict_raw_json_decoder
    .reject_prototype_mutation_keys,
  ["__proto__", "constructor", "prototype"],
);
assert.equal(
  security.request_digest.strict_raw_json_decoder
    .reject_duplicate_keys_at_any_depth,
  true,
);
assert.equal(
  security.request_digest.strict_raw_json_decoder
    .already_parsed_object_is_acceptable_evidence,
  false,
);
for (const scenario of executableSecurity.raw_entrypoint_cases) {
  assert(
    Object.hasOwn(
      security.request_digest.strict_raw_json_decoder.entry_points,
      scenario.entry_point,
    ),
    `${scenario.name} uses an unbound strict-decoding entry point`,
  );
  let outcome = "REJECT";
  try {
    const decoded = parseStrictJson(scenario.raw);
    const schemaAccepted = validatorFor(scenario.schema)(decoded);
    if (
      schemaAccepted &&
      scenario.entry_point === "HTTP_MUTATION_BODY"
    ) {
      const parsedDigest = canonicalRequestDigest({
        schema_version: "fit.platform.request-digest.v1",
        method: "POST",
        route_template: "/v1/operations/{operation_id}/confirm",
        path: decoded.path,
        query: decoded.query,
        body: decoded.body,
        user_id: "10000000-0000-4000-8000-000000000001",
        trading_account_id: "20000000-0000-4000-8000-000000000001",
      });
      const plainDigest = canonicalRequestDigest({
        schema_version: "fit.platform.request-digest.v1",
        method: "POST",
        route_template: "/v1/operations/{operation_id}/confirm",
        path: clone(decoded.path),
        query: clone(decoded.query),
        body: clone(decoded.body),
        user_id: "10000000-0000-4000-8000-000000000001",
        trading_account_id: "20000000-0000-4000-8000-000000000001",
      });
      assert.equal(parsedDigest, plainDigest, `${scenario.name} digest drift`);
    }
    outcome = schemaAccepted ? "ACCEPT" : "REJECT";
  } catch {
    outcome = "REJECT";
  }
  assert.equal(outcome, scenario.expected, scenario.name);
}

for (const vector of scenarios.source_key_vectors) {
  const canonical = canonicalIpBytes(vector.direct_peer_ip);
  assert.equal(canonical.toString("hex"), vector.canonical_ip_hex, vector.name);
  const digest = createHmac("sha256", vector.test_key_utf8)
    .update(canonical)
    .digest("hex");
  assert.equal(digest, vector.expected_hmac_sha256, vector.name);
}
assert.equal(
  canonicalIpBytes(scenarios.forwarding_header_case.direct_peer_ip).toString(
    "hex",
  ),
  scenarios.forwarding_header_case.expected_canonical_ip_hex,
  "forwarding header must not select source identity",
);
for (const scenario of scenarios.source_key_rotation_cases) {
  assert.equal(
    scenario.elapsed_seconds < security.source_key.prior_key_overlap_seconds,
    scenario.previous_key_accepted_for_correlation,
    `source-key overlap boundary ${scenario.elapsed_seconds}`,
  );
}

for (const scenario of scenarios.rate_limit_cases) {
  const group = rateLimits.groups[scenario.group];
  assert(group, `rate fixture references unknown group ${scenario.group}`);
  assert.equal(group.rate, scenario.rate, scenario.group);
  assert.equal(group.per_seconds, scenario.per_seconds, scenario.group);
  assert.equal(group.burst, scenario.burst, scenario.group);
  if (Object.hasOwn(scenario, "maximum_concurrent")) {
    assert.equal(
      group.maximum_concurrent,
      scenario.maximum_concurrent,
      scenario.group,
    );
  }
}
for (const [name, group] of Object.entries(rateLimits.groups)) {
  if (Object.hasOwn(group, "maximum_attempts")) {
    assert.equal(group.maximum_attempts, 1, `${name} must be one attempt`);
    const attempts = [0, 1].map((attempt) => attempt < group.maximum_attempts);
    assertDeepEqual(attempts, [true, false], `${name} compare-and-set boundary`);
    continue;
  }
  const atBurst = Array(group.burst + 1).fill(0);
  const burstDecisions = tokenBucketDecisions(group, atBurst);
  assert(
    burstDecisions.slice(0, group.burst).every(Boolean),
    `${name} denied inside burst`,
  );
  assert.equal(
    burstDecisions[group.burst],
    false,
    `${name} allowed above burst`,
  );
  const refillMs = (group.per_seconds / group.rate) * 1000;
  assert.equal(
    tokenBucketDecisions(group, [...Array(group.burst).fill(0), refillMs]).at(
      -1,
    ),
    true,
    `${name} did not refill one token`,
  );
  if (Object.hasOwn(group, "maximum_concurrent")) {
    let concurrent = 0;
    const acquire = () =>
      concurrent < group.maximum_concurrent ? ((concurrent += 1), true) : false;
    for (let index = 0; index < group.maximum_concurrent; index += 1) {
      assert(acquire(), `${name} denied within concurrency maximum`);
    }
    assert.equal(acquire(), false, `${name} exceeded concurrency maximum`);
    concurrent -= 1;
    assert(acquire(), `${name} failed to admit after release`);
  }
}
const sessionGroup = rateLimits.groups["authenticated-session"];
const sourceGroup = rateLimits.groups["authenticated-source"];
const sessionDenied = tokenBucketDecisions(
  sessionGroup,
  Array(sessionGroup.burst + 1).fill(0),
).at(-1);
const sourceAllowed = tokenBucketDecisions(sourceGroup, [0])[0];
assert.equal(
  sessionDenied && sourceAllowed,
  false,
  "applicable rate-limit groups must deny when either dimension denies",
);

for (const scenario of scenarios.enrollment_effect_cases) {
  const enrollment =
    scenario.purpose === "ADDITIONAL_DEVICE"
      ? security.enrollment.ordinary
      : security.enrollment.replacement;
  const expected =
    scenario.purpose === "ADDITIONAL_DEVICE" ? "PRESERVE" : "REVOKE_ALL";
  assert.equal(enrollment.prior_devices, expected, scenario.purpose);
  assert.equal(enrollment.prior_sessions, expected, scenario.purpose);
  assert.equal(enrollment.prior_refresh_families, expected, scenario.purpose);
  assert.equal(
    scenario.new_session_origin,
    "SERVER_RANDOM_INDEPENDENT",
    scenario.purpose,
  );
  const expectedFixtureEffect =
    scenario.purpose === "ADDITIONAL_DEVICE"
      ? "PRESERVED"
      : "REVOKED_BEFORE_NEW_SESSION";
  assert.equal(
    scenario.expected_prior_devices,
    expectedFixtureEffect,
    scenario.purpose,
  );
  assert.equal(
    scenario.expected_prior_sessions,
    expectedFixtureEffect,
    scenario.purpose,
  );
  assert.equal(
    scenario.expected_prior_families,
    expectedFixtureEffect,
    scenario.purpose,
  );
}
for (const scenario of scenarios.device_key_cases) {
  let outcome;
  if (scenario.operation === "DEVICE_ACTION" && scenario.key_revoked) {
    outcome = "REJECT_REVOKED_KEY";
  } else if (scenario.existing_owner === "NONE") {
    outcome = "REGISTER";
  } else if (scenario.existing_owner === scenario.requested_owner) {
    outcome = "REJECT_DUPLICATE_KEY";
  } else {
    outcome = "REJECT_CROSS_OWNER_REBIND";
  }
  assert.equal(outcome, scenario.expected, scenario.name);
}
for (const scenario of scenarios.session_fixation_cases) {
  assert.notEqual(
    scenario.presented_preauthentication_id,
    scenario.created_session_id,
    `${scenario.workflow} promoted a preauthentication identifier`,
  );
  assert.notEqual(
    scenario.presented_preauthentication_id,
    scenario.created_family_id,
    `${scenario.workflow} promoted a preauthentication identifier to family`,
  );
  assert.notEqual(
    scenario.created_session_id,
    scenario.created_family_id,
    `${scenario.workflow} reused a session ID as a family ID`,
  );
  const expectedPriorEffect = {
    LOGIN: "INVALIDATE_PREAUTHENTICATION_IDENTIFIER",
    ORDINARY_ENROLLMENT: "PRESERVE_EXISTING_AUTHENTICATED_SCOPE",
    REPLACEMENT_ENROLLMENT:
      "REVOKE_EXISTING_AUTHENTICATED_SCOPE_BEFORE_CREATE",
  }[scenario.workflow];
  assert.equal(
    scenario.prior_scope_effect,
    expectedPriorEffect,
    `${scenario.workflow} fixation boundary drifted`,
  );
}
assertDeepEqual(
  scenarios.revocation_deadline_cases.map(({ ingress }) => ingress).sort(),
  [...security.revocation.ingress].sort(),
  "revocation deadline fixtures must cover every ingress",
);
for (const scenario of scenarios.revocation_deadline_cases) {
  assert(
    scenario.elapsed_ms <= security.revocation.maximum_propagation_seconds * 1000,
    `${scenario.ingress} revocation proof exceeds the deadline`,
  );
  assert(
    scenario.expected.startsWith("DENY"),
    `${scenario.ingress} does not fail closed at the deadline`,
  );
}

for (const scenario of scenarios.wal_cases) {
  assert.equal(
    walOutcome(scenario),
    scenario.expected,
    `WAL case failed for age ${scenario.archive_age_seconds}`,
  );
}

const recoveryCase = scenarios.recovery_calculation_case;
assert.equal(
  recoveryCase.last_committed_sequence - recoveryCase.last_recovered_sequence,
  recoveryCase.expected_sequence_gap,
);
assert.equal(
  isoMilliseconds(recoveryCase.last_committed_at) -
    isoMilliseconds(recoveryCase.last_recovered_at),
  recoveryCase.expected_time_gap_ms,
);
assert.equal(
  isoMilliseconds(recoveryCase.verification_completed_at) -
    isoMilliseconds(recoveryCase.restore_invoked_at),
  recoveryCase.expected_rto_ms,
);

const recoveryFixture = validFixtureSet.cases.find(
  (fixture) => fixture.schema === "RecoveryEvidence",
).value;
assertDeepEqual(
  recoveryFixture.workload,
  {
    workload_id: recoveryPolicy.workload.workload_id,
    workload_manifest_sha256:
      recoveryPolicy.workload.workload_manifest_sha256,
    fixed_seed: recoveryPolicy.workload.fixed_seed,
    event_rate_per_second: recoveryPolicy.workload.event_rate_per_second,
    utc_clock_source: recoveryPolicy.workload.utc_clock_source,
  },
  "recovery evidence workload drifted from frozen policy",
);
const {
  workload_manifest_sha256: frozenWorkloadHash,
  ...frozenWorkloadDefinition
} = recoveryPolicy.workload;
assert.equal(
  sha256(jcsCanonicalize(frozenWorkloadDefinition)),
  frozenWorkloadHash,
  "recovery workload manifest hash drifted",
);
assert.equal(
  recoveryFixture.last_committed.sequence -
    recoveryFixture.last_recovered.sequence,
  recoveryFixture.observed_rpo.sequence_gap,
);
assert.equal(
  isoMilliseconds(recoveryFixture.last_committed.committed_at) -
    isoMilliseconds(recoveryFixture.last_recovered.committed_at),
  recoveryFixture.observed_rpo.time_gap_ms,
);
assert.equal(
  isoMilliseconds(recoveryFixture.restore.verification_completed_at) -
    isoMilliseconds(recoveryFixture.restore.invoked_at),
  recoveryFixture.observed_rto_ms,
);
const recoveryWithTarget = {
  ...clone(recoveryFixture),
  target_rto_ms: recoveryFixture.observed_rto_ms,
};
assert(
  !recoveryValidator(recoveryWithTarget),
  "recovery evidence must reject an SLA target presented beside measurements",
);
const recoveryWithFailedGate = clone(recoveryFixture);
recoveryWithFailedGate.verification_gates[0].status = "FAIL";
assert(
  !recoveryValidator(recoveryWithFailedGate),
  "PASS recovery evidence must reject a failed verification gate",
);
for (const mutate of [
  (value) => {
    value.workload.fixed_seed = 7;
  },
  (value) => {
    value.workload.event_rate_per_second = 1;
  },
  (value) => {
    value.observed_rpo.sequence_gap = 999;
  },
  (value) => {
    value.observed_rpo.time_gap_ms = 999;
  },
  (value) => {
    value.observed_rto_ms = 1;
  },
  (value) => {
    value.last_recovered.sequence = value.last_committed.sequence + 1;
  },
  (value) => {
    value.failure.injected_at = "2026-07-29T10:08:00Z";
  },
  (value) => {
    value.restore.verification_completed_at = "2026-07-29T10:05:59Z";
  },
]) {
  const adversarial = clone(recoveryFixture);
  mutate(adversarial);
  assert(
    !recoveryValidator(adversarial),
    "recovery evidence accepted drifted workload, arithmetic, or ordering",
  );
}
for (const failureKind of recoveryPolicy.fail_closed_conditions) {
  const falselyPassing = clone(recoveryFixture);
  falselyPassing.failure.kind = failureKind;
  assert(
    !recoveryValidator(falselyPassing),
    `${failureKind} accepted result=PASS`,
  );
  const failClosedWithNoFailedGate = clone(falselyPassing);
  failClosedWithNoFailedGate.result = "FAIL_CLOSED";
  delete failClosedWithNoFailedGate.observed_rto_ms;
  failClosedWithNoFailedGate.restore.verification_stopped_at =
    failClosedWithNoFailedGate.restore.verification_completed_at;
  failClosedWithNoFailedGate.restore.stop_event_id =
    failClosedWithNoFailedGate.restore.completion_event_id;
  failClosedWithNoFailedGate.restore.failure_reason_code =
    `${failureKind}_DETECTED`;
  delete failClosedWithNoFailedGate.restore.verification_completed_at;
  delete failClosedWithNoFailedGate.restore.completion_event_id;
  assert(
    !recoveryValidator(failClosedWithNoFailedGate),
    `${failureKind} accepted FAIL_CLOSED without a failed verification gate`,
  );
  failClosedWithNoFailedGate.verification_gates[0].status = "FAIL";
  assert(
    recoveryValidator(failClosedWithNoFailedGate),
    `${failureKind} rejected bound fail-closed evidence`,
  );
  const falseRto = clone(failClosedWithNoFailedGate);
  falseRto.observed_rto_ms = 90_000;
  assert(
    !recoveryValidator(falseRto),
    `${failureKind} falsely reported observed RTO after failed verification`,
  );
}
for (const mutate of [
  (value) => {
    value.restore.completion_event_id = value.restore.start_event_id;
  },
  (value) => {
    value.components = [value.components[0], clone(value.components[0])];
    value.components[1].artifact_sha256 = "1".repeat(64);
  },
  (value) => {
    value.components = [value.components[0]];
  },
]) {
  const adversarial = clone(recoveryFixture);
  mutate(adversarial);
  assert(
    !recoveryValidator(adversarial),
    "recovery evidence accepted ambiguous restore markers or components",
  );
}
for (const malformed of [{}, { schema_version: "fit.platform.recovery-evidence.v1" }]) {
  assert.doesNotThrow(
    () => recoveryValidator(malformed),
    "RecoveryEvidence validator threw on malformed input",
  );
  assert(!recoveryValidator(malformed));
}

const validEventEnvelope = validFixtureSet.cases.find(
  ({ schema }) => schema === "EventEnvelope",
).value;
for (const mutate of [
  (value) => {
    value.payload.data.decision = "REJECT";
  },
  (value) => {
    value.payload_schema_version = "fit.platform.changed.v1";
  },
  (value) => {
    value.payload_digest = "0".repeat(64);
  },
]) {
  const adversarial = clone(validEventEnvelope);
  mutate(adversarial);
  assert(
    !validatorFor("EventEnvelope")(adversarial),
    "event envelope accepted an unbound payload mutation",
  );
}
const nonexistentPayloadSchema = clone(validEventEnvelope);
nonexistentPayloadSchema.payload_schema_version =
  "fit.platform.nonexistent-payload.v1";
nonexistentPayloadSchema.payload.schema_version =
  nonexistentPayloadSchema.payload_schema_version;
nonexistentPayloadSchema.payload_digest = sha256(
  jcsCanonicalize(nonexistentPayloadSchema.payload),
);
assert(
  !validatorFor("EventEnvelope")(nonexistentPayloadSchema),
  "event envelope accepted an unregistered payload schema",
);
for (const mutate of [
  (value) => {
    value.payload.subject = "fit.platform.v1.system.wal-archive-interrupted";
  },
  (value) => {
    value.payload.event_kind = "WAL_ARCHIVE_INTERRUPTED";
  },
  (value) => {
    value.payload.scope.trading_account_id =
      "20000000-0000-4000-8000-000000000099";
  },
]) {
  const adversarial = clone(validEventEnvelope);
  mutate(adversarial);
  adversarial.payload_digest = sha256(jcsCanonicalize(adversarial.payload));
  assert(
    !validatorFor("EventEnvelope")(adversarial),
    "event envelope accepted recomputed payload identity drift",
  );
}
for (const malformed of [{}, { schema_version: "fit.platform.event-envelope.v1" }]) {
  assert.doesNotThrow(
    () => validatorFor("EventEnvelope")(malformed),
    "EventEnvelope validator threw on malformed input",
  );
  assert(!validatorFor("EventEnvelope")(malformed));
}

const canonicalGolden = jcsCanonicalize(golden.request_digest.value);
assert.equal(canonicalGolden, golden.request_digest.canonical_utf8);
assert.equal(sha256(canonicalGolden), golden.request_digest.sha256);
assert.equal(
  canonicalRequestDigest(golden.request_digest.value),
  golden.request_digest.sha256,
);
for (const field of requestDigestBoundFields) {
  const mutation = clone(golden.request_digest.value);
  if (typeof mutation[field] === "string") {
    mutation[field] =
      field === "method"
        ? "PATCH"
        : field.endsWith("_id")
          ? "21000000-0000-4000-8000-000000000001"
          : `${mutation[field]}-changed`;
  } else {
    mutation[field] = { ...mutation[field], changed: true };
  }
  assert.notEqual(
    canonicalRequestDigest(mutation),
    golden.request_digest.sha256,
    `request digest must bind ${field}`,
  );
}
const excludedMutation = {
  ...golden.request_digest.value,
  authorization: "synthetic-excluded-value",
  cookie: "synthetic-excluded-value",
  forwarding_headers: { forwarded: "synthetic-excluded-value" },
  client_identity_headers: { user: "synthetic-excluded-value" },
};
assert.equal(
  canonicalRequestDigest(excludedMutation),
  golden.request_digest.sha256,
  "excluded transport metadata must not enter request digest",
);
const canonicalPayload = jcsCanonicalize(golden.payload_digest.value);
assert.equal(canonicalPayload, golden.payload_digest.canonical_utf8);
assert.equal(sha256(canonicalPayload), golden.payload_digest.sha256);
for (const field of Object.keys(golden.payload_digest.value)) {
  const mutation = clone(golden.payload_digest.value);
  mutation[field] =
    typeof mutation[field] === "object"
      ? { ...mutation[field], changed: true }
      : `${mutation[field]}-changed`;
  assert.notEqual(
    sha256(jcsCanonicalize(mutation)),
    golden.payload_digest.sha256,
    `payload digest must bind ${field}`,
  );
}
const rawPublicKey = Buffer.from(
  golden.ed25519_proofs.public_key.slice("ed25519-public:".length),
  "hex",
);
assert.equal(rawPublicKey.length, 32);
assert.equal(
  `ed25519:${sha256(rawPublicKey)}`,
  golden.ed25519_proofs.public_key_fingerprint,
  "Ed25519 fingerprint derivation drifted",
);
const publicKey = createPublicKey({
  key: Buffer.concat([
    Buffer.from("302a300506032b6570032100", "hex"),
    rawPublicKey,
  ]),
  format: "der",
  type: "spki",
});
for (const vector of golden.ed25519_proofs.vectors) {
  assert(
    validatorFor(vector.challenge_schema)(vector.challenge),
    `${vector.name} challenge is not contract-valid`,
  );
  const canonical = jcsCanonicalize(vector.challenge);
  assert.equal(canonical, vector.canonical_utf8, vector.name);
  const signedBytes = Buffer.concat([
    Buffer.from(vector.challenge.domain, "utf8"),
    Buffer.from([0]),
    Buffer.from(canonical, "utf8"),
  ]);
  assert.equal(signedBytes.toString("hex"), vector.signed_bytes_hex, vector.name);
  const signature = Buffer.from(
    vector.signature.slice("ed25519-signature:".length),
    "hex",
  );
  assert(
    verifySignature(null, signedBytes, publicKey, signature),
    `${vector.name} signature did not verify`,
  );
  for (const field of Object.keys(vector.challenge)) {
    const mutation = clone(vector.challenge);
    mutation[field] =
      typeof mutation[field] === "boolean"
        ? !mutation[field]
        : `${mutation[field]}-mutated`;
    const mutatedCanonical = jcsCanonicalize(mutation);
    const mutatedBytes = Buffer.concat([
      Buffer.from(mutation.domain, "utf8"),
      Buffer.from([0]),
      Buffer.from(mutatedCanonical, "utf8"),
    ]);
    assert(
      !verifySignature(null, mutatedBytes, publicKey, signature),
      `${vector.name} signature did not bind ${field}`,
    );
  }
  const tamperedSignature = Buffer.from(signature);
  tamperedSignature[0] ^= 1;
  assert(
    !verifySignature(null, signedBytes, publicKey, tamperedSignature),
    `${vector.name} accepted a tampered signature`,
  );
}
const enrollmentProofVector = golden.ed25519_proofs.vectors.find(
  ({ challenge_schema }) => challenge_schema === "EnrollmentChallenge",
);
const deviceActionProofVector = golden.ed25519_proofs.vectors.find(
  ({ challenge_schema }) => challenge_schema === "DeviceActionChallenge",
);
assertDeepEqual(validEnrollmentChallenge, enrollmentProofVector.challenge);
assertDeepEqual(
  validFixtureSet.cases.find(
    ({ schema }) => schema === "DeviceActionChallenge",
  ).value,
  deviceActionProofVector.challenge,
);
const validEnrollmentCompletion = validFixtureSet.cases.find(
  ({ schema }) => schema === "EnrollmentCompletionInput",
).value;
assert.equal(
  validEnrollmentCompletion.candidate_public_key,
  golden.ed25519_proofs.public_key,
);
assert.equal(validEnrollmentCompletion.signature, enrollmentProofVector.signature);
assert.equal(
  validFixtureSet.cases.find(({ schema }) => schema === "Device").value
    .public_key_fingerprint,
  golden.ed25519_proofs.public_key_fingerprint,
);
assert.equal(
  validFixtureSet.cases.find(
    ({ schema, name }) =>
      schema === "DeviceEnrollment" && name.startsWith("ordinary"),
  ).value.candidate_public_key_fingerprint,
  golden.ed25519_proofs.public_key_fingerprint,
);
assert.equal(
  validFixtureSet.cases.find(
    ({ schema }) => schema === "DeviceActionProofInput",
  ).value.signature,
  deviceActionProofVector.signature,
);

assertDeepEqual(
  {
    profile_id: golden.argon2id_production.profile_id,
    minimum_memory_kib: golden.argon2id_production.memory_kib,
    minimum_iterations: golden.argon2id_production.iterations,
    parallelism: golden.argon2id_production.parallelism,
    salt_bytes: Buffer.from(
      golden.argon2id_production.salt_hex,
      "hex",
    ).length,
    result_bytes: Buffer.from(
      golden.argon2id_production.result_hex,
      "hex",
    ).length,
    version: golden.argon2id_production.version,
  },
  {
    profile_id: security.password_hashing.production_profile.profile_id,
    minimum_memory_kib:
      security.password_hashing.production_profile.minimum_memory_kib,
    minimum_iterations:
      security.password_hashing.production_profile.minimum_iterations,
    parallelism:
      security.password_hashing.production_profile.parallelism,
    salt_bytes: security.password_hashing.production_profile.salt_bytes,
    result_bytes: security.password_hashing.production_profile.result_bytes,
    version: security.password_hashing.version,
  },
  "Argon2id production vector metadata drifted",
);
assert.equal(
  golden.argon2id_production.result_hex,
  "79cba77cff303e01dcae3bd6a16dddd07b734b1b91eda7c7d42c574101d9a920",
  "Argon2id production vector digest drifted",
);
const nodeCrypto = await import("node:crypto");
let argonVectorExecution = "NODE_CRYPTO_ARGON2_UNAVAILABLE";
if (typeof nodeCrypto.argon2Sync === "function") {
  const result = nodeCrypto.argon2Sync("argon2id", {
    message: Buffer.from(golden.argon2id_production.password_utf8, "utf8"),
    nonce: Buffer.from(golden.argon2id_production.salt_hex, "hex"),
    parallelism: golden.argon2id_production.parallelism,
    tagLength: golden.argon2id_production.result_bytes,
    memory: golden.argon2id_production.memory_kib,
    passes: golden.argon2id_production.iterations,
  });
  assert.equal(
    result.toString("hex"),
    golden.argon2id_production.result_hex,
    "Node crypto Argon2id recomputation drifted",
  );
  argonVectorExecution = "VERIFIED_BY_NODE_CRYPTO_ARGON2";
} else {
  assert(
    Number(process.versions.node.split(".")[0]) < 24,
    "Node 24+ contract runtime must expose crypto.argon2Sync",
  );
}

const expectedUnauthenticatedRoutes = [
  "GET /health/live",
  "GET /health/ready",
  "POST /v1/auth/device-enrollments/complete",
  "POST /v1/auth/device-enrollments/start",
  "POST /v1/auth/device-replacements/complete",
  "POST /v1/auth/device-replacements/start",
  "POST /v1/auth/login",
  "POST /v1/auth/refresh",
];
assertDeepEqual(
  rateLimits.unauthenticated_allowlist.map(({ route }) => route).sort(),
  expectedUnauthenticatedRoutes,
  "unauthenticated allowlist drifted",
);
assert.equal(
  rateLimits.default_route_policy,
  "AUTHENTICATE_BEFORE_HANDLER_DISPATCH",
);
assertDeepEqual(rateLimits.groups["password-starts-source"], {
  scope: "SOURCE",
  rate: 10,
  per_seconds: 60,
  burst: 3,
  aggregate_routes: true,
});
assertDeepEqual(rateLimits.groups["refresh-source"], {
  scope: "SOURCE",
  rate: 30,
  per_seconds: 60,
  burst: 5,
});
assertDeepEqual(rateLimits.groups["refresh-family"], {
  scope: "REFRESH_FAMILY",
  rate: 10,
  per_seconds: 60,
  burst: 3,
});
assertDeepEqual(rateLimits.groups["authenticated-session"], {
  scope: "SESSION",
  rate: 20,
  per_seconds: 1,
  burst: 40,
});
assertDeepEqual(rateLimits.groups["authenticated-source"], {
  scope: "SOURCE",
  rate: 50,
  per_seconds: 1,
  burst: 100,
});
assertDeepEqual(rateLimits.groups["websocket-session"], {
  scope: "SESSION",
  rate: 2,
  per_seconds: 1,
  burst: 2,
  maximum_concurrent: 5,
});
assertDeepEqual(rateLimits.groups["websocket-source"], {
  scope: "SOURCE",
  rate: 10,
  per_seconds: 1,
  burst: 20,
  maximum_concurrent: 20,
});

const exactSubjects = [
  "fit.platform.v1.auth-security.device-enrolled",
  "fit.platform.v1.auth-security.device-replaced",
  "fit.platform.v1.auth-security.device-revoked",
  "fit.platform.v1.auth-security.enrollment-challenge-issued",
  "fit.platform.v1.auth-security.enrollment-proof-rejected",
  "fit.platform.v1.auth-security.login-account-locked",
  "fit.platform.v1.auth-security.login-source-locked",
  "fit.platform.v1.auth-security.login-succeeded",
  "fit.platform.v1.auth-security.refresh-reuse-detected",
  "fit.platform.v1.auth-security.session-revoked",
  "fit.platform.v1.notification.device-enrolled",
  "fit.platform.v1.notification.device-replaced",
  "fit.platform.v1.notification.device-revoked",
  "fit.platform.v1.notification.login-account-locked",
  "fit.platform.v1.notification.login-source-locked",
  "fit.platform.v1.notification.refresh-reuse-detected",
  "fit.platform.v1.notification.session-revoked",
  "fit.platform.v1.notification.wal-archive-interrupted",
  "fit.platform.v1.owner.owner-mutation-committed",
  "fit.platform.v1.system.wal-archive-interrupted",
];
assertDeepEqual(nats.stream.subjects, exactSubjects);
assertDeepEqual(
  nats.subject_bindings.map(({ subject }) => subject),
  exactSubjects,
  "each exact subject requires one binding",
);
assertDeepEqual(nats.event_semantics.ordering, {
  scope: "PER_AGGREGATE",
  fields: ["aggregate_id", "aggregate_version"],
  global_order_claimed: false,
});
assert.equal(nats.event_semantics.redelivery.delivery, "AT_LEAST_ONCE");
assert.equal(
  nats.event_semantics.redelivery.same_event_id_different_digest,
  "REJECT_AND_ALERT",
);
assert.equal(
  nats.event_semantics.compatibility.same_major_changes,
  "ADDITIVE_ONLY",
);
assert.equal(
  nats.event_semantics.compatibility.breaking_change,
  "NEW_VERSIONED_SUBJECT_AND_REVIEW",
);
assertDeepEqual(
  nats.principals.map(({ name }) => name).sort(),
  [
    "internal-notification-consumer",
    "outbox-publisher",
    "platform-consumer",
  ],
);
for (const principal of nats.principals) {
  for (const subject of [...principal.publish, ...principal.subscribe]) {
    assert(
      exactSubjects.includes(subject),
      `${principal.name} has unknown subject ${subject}`,
    );
    assert(
      !subject.includes("*") && !subject.includes(">"),
      `${principal.name} has a wildcard`,
    );
  }
  assert.equal(principal.jetstream_admin, false);
  assertDeepEqual(principal.identity_claims, []);
}
const publisher = nats.principals.find(
  ({ name }) => name === "outbox-publisher",
);
assertDeepEqual(publisher.publish, exactSubjects);
assertDeepEqual(publisher.subscribe, []);
for (const consumer of nats.principals.filter(
  ({ name }) => name !== "outbox-publisher",
)) {
  assertDeepEqual(consumer.publish, []);
  assertDeepEqual(
    consumer.subscribe,
    nats.subject_bindings
      .filter((binding) => binding.consumer === consumer.name)
      .map(({ subject }) => subject),
    `${consumer.name} subject allowlist drifted`,
  );
}
assert.equal(nats.api_service_has_nats_credential, false);
assert.equal(nats.user_or_account_claims_allowed, false);

const validateEventEnvelope = validatorFor("EventEnvelope");
const validateAuditIntent = validatorFor("AuditIntent");
const validateAuditEvent = validatorFor("AuditEvent");
const validateNotification = validatorFor("Notification");
const validateOutbox = validatorFor("OutboxRecord");
const causallyValidOutbox = validFixtureSet.cases.find(
  ({ schema }) => schema === "OutboxRecord",
).value;
const futureEventOutbox = clone(causallyValidOutbox);
futureEventOutbox.event.occurred_at = "2026-07-30T10:00:00Z";
assert(
  !validateOutbox(futureEventOutbox),
  "Outbox accepted an event occurring after the Outbox record was created",
);
const notificationByKind = {
  LOGIN_SOURCE_LOCKED: {
    severity: "WARNING",
    message_code: "security.login.source_locked",
    message_args: { lock_seconds: 900 },
  },
  LOGIN_ACCOUNT_LOCKED: {
    severity: "WARNING",
    message_code: "security.login.account_locked",
    message_args: { lock_seconds: 900 },
  },
  DEVICE_ENROLLED: {
    severity: "INFO",
    message_code: "security.device.enrolled",
    message_args: {
      device_id: "30000000-0000-4000-8000-000000000001",
    },
  },
  DEVICE_REPLACED: {
    severity: "CRITICAL",
    message_code: "security.device.replaced",
    message_args: {
      device_id: "30000000-0000-4000-8000-000000000001",
    },
  },
  DEVICE_REVOKED: {
    severity: "WARNING",
    message_code: "security.device.revoked",
    message_args: {
      device_id: "30000000-0000-4000-8000-000000000001",
    },
  },
  SESSION_REVOKED: {
    severity: "INFO",
    message_code: "security.session.revoked",
    message_args: {
      session_id: "40000000-0000-4000-8000-000000000001",
    },
  },
  REFRESH_REUSE_DETECTED: {
    severity: "CRITICAL",
    message_code: "security.refresh.reuse_detected",
    message_args: {
      session_id: "40000000-0000-4000-8000-000000000001",
    },
  },
  WAL_ARCHIVE_INTERRUPTED: {
    severity: "CRITICAL",
    message_code: "system.wal.archive_interrupted",
    message_args: {
      incident_id: "25000000-0000-4000-8000-000000000003",
      archive_age_seconds: 61,
    },
  },
};
for (const [chainIndex, chain] of linkageChains.chains.entries()) {
  const actor =
    chain.scope.type === "SYSTEM"
      ? { type: "SYSTEM", id: "wal-monitor" }
      : chain.scope.type === "OWNER"
        ? { type: "USER", id: "synthetic-owner" }
        : { type: "SERVICE", id: "auth-service" };
  const auditIntent = {
    schema_version: "fit.platform.audit-intent.v1",
    kind: chain.kind,
    actor,
    scope: chain.scope,
    trigger: chain.trigger,
    causation_id: chain.causation_id,
    correlation_id: chain.correlation_id,
    [chain.operation_link_field]: chain.operation_id,
  };
  assert(
    validateAuditIntent(auditIntent),
    `${chain.name} AuditIntent invalid: ${ajv.errorsText(
      validateAuditIntent.errors,
    )}`,
  );
  const auditEvent = {
    ...auditIntent,
    schema_version: "fit.platform.audit-event.v1",
    event_id: chain.audit_event_id,
    occurred_at: "2026-07-29T10:00:01Z",
    payload_digest: sha256(jcsCanonicalize(auditIntent)),
  };
  assert(
    validateAuditEvent(auditEvent),
    `${chain.name} AuditEvent invalid: ${ajv.errorsText(
      validateAuditEvent.errors,
    )}`,
  );
  const durableRecords = [
    {
      recordType: "audit",
      recordId: chain.audit_event_id,
      subject: chain.audit_subject,
      record: auditEvent,
    },
  ];
  if (chain.notification_id !== null) {
    const details = notificationByKind[chain.kind];
    assert(details, `${chain.name} lacks notification mapping`);
    const notificationIntent = {
      schema_version: "fit.platform.notification-intent.v1",
      kind: chain.kind,
      severity: details.severity,
      scope: chain.scope,
      message_code: details.message_code,
      message_args: details.message_args,
      causation_id: chain.causation_id,
      correlation_id: chain.correlation_id,
    };
    assert(
      validateNotificationIntent(notificationIntent),
      `${chain.name} NotificationIntent invalid: ${ajv.errorsText(
        validateNotificationIntent.errors,
      )}`,
    );
    const notification = {
      ...notificationIntent,
      schema_version: "fit.platform.notification.v1",
      notification_id: chain.notification_id,
      occurred_at: "2026-07-29T10:00:01Z",
      payload_digest: sha256(jcsCanonicalize(notificationIntent)),
    };
    assert(
      validateNotification(notification),
      `${chain.name} Notification invalid: ${ajv.errorsText(
        validateNotification.errors,
      )}`,
    );
    durableRecords.push({
      recordType: "notification",
      recordId: chain.notification_id,
      subject: chain.notification_subject,
      record: notification,
    });
  }
  const outboxRecords = durableRecords.map((durable, recordIndex) => {
    const payload = {
      schema_version: "fit.platform.event-payload.v1",
      subject: durable.subject,
      event_kind: chain.kind,
      scope: chain.scope,
      data: {
        record_type: durable.recordType,
        record_id: durable.recordId,
        record: durable.record,
      },
    };
    const event = {
      schema_version: "fit.platform.event-envelope.v1",
      subject: durable.subject,
      stream: "FIT_PLATFORM_V1",
      event_id: `26000000-0000-4000-8000-${String(
        chainIndex * 2 + recordIndex + 1,
      ).padStart(12, "0")}`,
      event_kind: chain.kind,
      scope: chain.scope,
      aggregate_type: chain.aggregate_type,
      aggregate_id: chain.aggregate_id,
      aggregate_version: recordIndex + 1,
      causation_id: chain.causation_id,
      correlation_id: chain.correlation_id,
      occurred_at: "2026-07-29T10:00:01Z",
      payload_schema_version: payload.schema_version,
      payload_digest: sha256(jcsCanonicalize(payload)),
      payload,
    };
    const outbox = {
      schema_version: "fit.platform.outbox-record.v1",
      outbox_id: `27000000-0000-4000-8000-${String(
        chainIndex * 2 + recordIndex + 1,
      ).padStart(12, "0")}`,
      event,
      created_at: "2026-07-29T10:00:01Z",
      publication_state: "PENDING",
      retention: "NEVER_DELETE_IN_PHASE_1",
    };
    assert(
      validateOutbox(outbox),
      `${chain.name} Outbox invalid: ${ajv.errorsText(validateOutbox.errors)}`,
    );
    return outbox;
  });
  assert.equal(
    outboxRecords.length,
    chain.expected_outbox_records,
    `${chain.name} outbox count`,
  );
  assertDeepEqual(
    outboxRecords.map(({ event }) => event.payload.data.record_id),
    durableRecords.map(({ recordId }) => recordId),
    `${chain.name} could not rebuild durable links from Outbox payloads`,
  );
  assertDeepEqual(
    outboxRecords.map(({ event }) => event.payload.data.record),
    durableRecords.map(({ record }) => record),
    `${chain.name} could not rebuild complete durable records from Outbox`,
  );
  for (const { event } of outboxRecords) {
    assert.equal(event.causation_id, chain.causation_id, chain.name);
    assert.equal(event.correlation_id, chain.correlation_id, chain.name);
    assertDeepEqual(event.scope, chain.scope, chain.name);
  }
  assertDeepEqual(
    chain.recovery_projection.audit_event_ids,
    [auditEvent.event_id],
    `${chain.name} recovery audit linkage`,
  );
  assertDeepEqual(
    chain.recovery_projection.notification_ids,
    durableRecords
      .filter(({ recordType }) => recordType === "notification")
      .map(({ recordId }) => recordId),
    `${chain.name} recovery notification linkage`,
  );
  assert.equal(chain.recovery_projection.source, "POSTGRESQL_OUTBOX");
  assert.equal(chain.recovery_projection.scope_type, chain.scope.type);
  assert.equal(chain.recovery_projection.phantom_owner, false);
  assert.equal(chain.trigger_context.phantom_owner_allowed, false);
  if (chain.trigger_context.identifier_resolved === false) {
    assert(!Object.hasOwn(chain.scope, "user_id"), chain.name);
    assert(!Object.hasOwn(chain.scope, "trading_account_id"), chain.name);
  } else if (
    chain.trigger_context.identifier_resolved === true &&
    chain.scope.type === "AUTH_SECURITY"
  ) {
    assert(Object.hasOwn(chain.scope, "user_id"), chain.name);
  } else if (chain.scope.type === "OWNER") {
    assert(Object.hasOwn(chain.scope, "user_id"), chain.name);
    assert(Object.hasOwn(chain.scope, "trading_account_id"), chain.name);
  }
}

for (const [index, binding] of nats.subject_bindings.entries()) {
  const scope =
    binding.scope === "OWNER"
      ? {
          type: "OWNER",
          user_id: "10000000-0000-4000-8000-000000000001",
          trading_account_id: "20000000-0000-4000-8000-000000000001",
        }
      : binding.scope === "AUTH_SECURITY"
        ? {
            type: "AUTH_SECURITY",
            source_key:
              "src_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
            ...([
              "LOGIN_SUCCEEDED",
              "LOGIN_ACCOUNT_LOCKED",
              "ENROLLMENT_CHALLENGE_ISSUED",
              "ENROLLMENT_PROOF_REJECTED",
            ].includes(binding.event_kind)
              ? { user_id: "10000000-0000-4000-8000-000000000001" }
              : {}),
          }
        : { type: "SYSTEM" };
  const recordId = `19000000-0000-4000-8${String(index).padStart(3, "0")}-000000000001`;
  const causationId = "17000000-0000-4000-8000-000000000001";
  const correlationId = "18000000-0000-4000-8000-000000000001";
  const recordIsNotification =
    binding.consumer === "internal-notification-consumer";
  let durableRecord;
  if (recordIsNotification) {
    const details = notificationByKind[binding.event_kind];
    assert(details, `missing notification mapping for ${binding.event_kind}`);
    durableRecord = {
      schema_version: "fit.platform.notification.v1",
      notification_id: recordId,
      kind: binding.event_kind,
      severity: details.severity,
      scope,
      message_code: details.message_code,
      message_args: details.message_args,
      causation_id: causationId,
      correlation_id: correlationId,
      occurred_at: "2026-07-29T10:00:01Z",
      payload_digest: "2".repeat(64),
    };
  } else {
    durableRecord = {
      schema_version: "fit.platform.audit-event.v1",
      event_id: recordId,
      kind: binding.event_kind,
      actor:
        binding.scope === "SYSTEM"
          ? { type: "SYSTEM", id: "system-monitor" }
          : binding.scope === "OWNER"
            ? { type: "USER", id: "synthetic-owner" }
            : { type: "SERVICE", id: "auth-service" },
      scope,
      trigger: binding.scope === "SYSTEM" ? "BACKGROUND" : "REQUEST",
      causation_id: causationId,
      correlation_id: correlationId,
      ...(binding.scope === "SYSTEM"
        ? { system_operation_id: causationId }
        : { request_id: causationId }),
      ...([
        "DEVICE_ENROLLED",
        "DEVICE_REPLACED",
        "DEVICE_REVOKED",
      ].includes(binding.event_kind)
        ? { device_id: "30000000-0000-4000-8000-000000000001" }
        : {}),
      ...(["SESSION_REVOKED", "REFRESH_REUSE_DETECTED"].includes(
        binding.event_kind,
      )
        ? { session_id: "40000000-0000-4000-8000-000000000001" }
        : {}),
      occurred_at: "2026-07-29T10:00:01Z",
      payload_digest: "2".repeat(64),
    };
  }
  const payload = {
    schema_version: "fit.platform.event-payload.v1",
    subject: binding.subject,
    event_kind: binding.event_kind,
    scope,
    data: {
      record_type: recordIsNotification ? "notification" : "audit",
      record_id: recordId,
      record: durableRecord,
    },
  };
  const envelope = {
    schema_version: "fit.platform.event-envelope.v1",
    subject: binding.subject,
    stream: "FIT_PLATFORM_V1",
    event_id: `15000000-0000-4000-8${String(index).padStart(3, "0")}-000000000001`,
    event_kind: binding.event_kind,
    scope,
    aggregate_type: "SYNTHETIC_AGGREGATE",
    aggregate_id: "16000000-0000-4000-8000-000000000001",
    aggregate_version: 1,
    causation_id: causationId,
    correlation_id: correlationId,
    occurred_at: "2026-07-29T10:00:01Z",
    payload_schema_version: "fit.platform.event-payload.v1",
    payload_digest: sha256(jcsCanonicalize(payload)),
    payload,
  };
  assert(
    validateEventEnvelope(envelope),
    `subject binding failed ${binding.subject}: ${ajv.errorsText(
      validateEventEnvelope.errors,
    )}`,
  );
  const wrongKind =
    binding.event_kind === "OWNER_MUTATION_COMMITTED"
      ? "LOGIN_SOURCE_LOCKED"
      : "OWNER_MUTATION_COMMITTED";
  assert(
    !validateEventEnvelope({ ...envelope, event_kind: wrongKind }),
    `subject accepted wrong event kind ${binding.subject}`,
  );
  if (
    [
      "LOGIN_SUCCEEDED",
      "LOGIN_ACCOUNT_LOCKED",
      "ENROLLMENT_CHALLENGE_ISSUED",
      "ENROLLMENT_PROOF_REJECTED",
    ].includes(binding.event_kind)
  ) {
    const unresolvedAccountScope = clone(envelope);
    delete unresolvedAccountScope.scope.user_id;
    assert(
      !validateEventEnvelope(unresolvedAccountScope),
      `${binding.subject} accepted unresolved account-lock scope`,
    );
  }
}

assert.equal(transactions.authority.system_of_record, "POSTGRESQL");
assert.equal(transactions.authority.nats, "DERIVED_DELIVERY_ONLY");
assert.equal(
  transactions.outbox_retention,
  "NEVER_DELETE_BUSINESS_EVENTS_IN_PHASE_1",
);
assert.equal(
  transactions.idempotency.same_scope_same_key_different_digest,
  "IDEMPOTENCY_CONFLICT",
);
const transactionEventContracts = [
  {
    workflow: "successful_login",
    kind: "LOGIN_SUCCEEDED",
    subject: "fit.platform.v1.auth-security.login-succeeded",
  },
  {
    workflow: "real_enrollment_challenge_creation",
    kind: "ENROLLMENT_CHALLENGE_ISSUED",
    subject: "fit.platform.v1.auth-security.enrollment-challenge-issued",
  },
  {
    workflow: "failed_enrollment_proof",
    kind: "ENROLLMENT_PROOF_REJECTED",
    subject: "fit.platform.v1.auth-security.enrollment-proof-rejected",
  },
];
for (const eventContract of transactionEventContracts) {
  const workflow = transactions.workflows.find(
    ({ name }) => name === eventContract.workflow,
  );
  assert.equal(workflow.audit_event_kind, eventContract.kind);
  assert.equal(workflow.outbox_subject, eventContract.subject);
  assert(
    platformSchema.$defs.AuditCore.properties.kind.enum.includes(
      eventContract.kind,
    ),
    `${eventContract.workflow} audit kind is not schema-expressible`,
  );
  assert(
    platformSchema.$defs.EventEnvelope.properties.event_kind.enum.includes(
      eventContract.kind,
    ),
    `${eventContract.workflow} event kind is not schema-expressible`,
  );
  assert(
    platformSchema.$defs.EventEnvelope.properties.subject.enum.includes(
      eventContract.subject,
    ),
    `${eventContract.workflow} subject is not schema-expressible`,
  );
  assert(
    nats.subject_bindings.some(
      ({ subject, event_kind: eventKind }) =>
        subject === eventContract.subject && eventKind === eventContract.kind,
    ),
    `${eventContract.workflow} subject lacks an exact NATS binding`,
  );
  const auditProbe = {
    schema_version: "fit.platform.audit-intent.v1",
    kind: eventContract.kind,
    actor: { type: "SERVICE", id: "auth-service" },
    scope: {
      type: "AUTH_SECURITY",
      source_key:
        "src_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
      user_id: "10000000-0000-4000-8000-000000000001",
    },
    trigger: "REQUEST",
    causation_id: "80000000-0000-4000-8000-000000000001",
    correlation_id: "b0000000-0000-4000-8000-000000000001",
    request_id: "21000000-0000-4000-8000-000000000010",
  };
  assert(
    validateAuditIntent(auditProbe),
    `${eventContract.workflow} cannot construct a valid AuditIntent: ${ajv.errorsText(
      validateAuditIntent.errors,
    )}`,
  );
}
const ownerMutationWorkflow = transactions.workflows.find(
  ({ name }) => name === "owner_mutation",
);
assert(!Object.hasOwn(ownerMutationWorkflow, "conditional_same_transaction"));
assert(!Object.hasOwn(ownerMutationWorkflow, "notification_event_kind"));
const successfulLoginWorkflow = transactions.workflows.find(
  ({ name }) => name === "successful_login",
);
const realEnrollmentStartWorkflow = transactions.workflows.find(
  ({ name }) => name === "real_enrollment_challenge_creation",
);
for (const requiredStep of [
  "lock_source_throttle_dimension",
  "prune_failures_outside_rolling_window",
  "verify_neither_dimension_delayed_or_locked",
  "verify_password",
  "clear_resolved_user_failures_only",
  "preserve_source_failures",
]) {
  for (const workflow of [
    successfulLoginWorkflow,
    realEnrollmentStartWorkflow,
  ]) {
    assert(
      workflow.single_postgresql_transaction.includes(requiredStep),
      `${workflow.name} transaction lacks ${requiredStep}`,
    );
  }
}
const passwordFailureWorkflow = transactions.workflows.find(
  ({ name }) => name === "password_failure",
);
assert.equal(
  passwordFailureWorkflow.rolling_window_seconds,
  security.password_throttle.window_seconds,
);
for (const requiredStep of [
  "lock_source_throttle_dimension",
  "lock_resolved_user_throttle_dimension_if_present",
  "prune_failures_outside_rolling_window",
  "record_source_failure",
  "record_resolved_user_failure_if_present",
  "set_next_allowed_or_lock_each_dimension",
]) {
  assert(
    passwordFailureWorkflow.single_postgresql_transaction.includes(requiredStep),
    `password failure transaction lacks ${requiredStep}`,
  );
}
const natsConsumerWorkflow = transactions.workflows.find(
  ({ name }) => name === "nats_consumer_business_effect",
);
assert.equal(natsConsumerWorkflow.ack_after_commit, true);
const outboxPublishWorkflow = transactions.workflows.find(
  ({ name }) => name === "outbox_publish",
);
assertDeepEqual(outboxPublishWorkflow.broker_phase, [
  "publish_exact_event_id_and_payload_digest",
  "receive_broker_ack",
]);
assert.equal(outboxPublishWorkflow.duplicate_publish_allowed, true);
assert.equal(outboxPublishWorkflow.duplicate_business_effect_allowed, false);
assert.equal(
  outboxPublishWorkflow.delete_business_event_allowed_in_phase_1,
  false,
);
assert.equal(
  failureInjection.schema_version,
  "fit.platform.failure-injection.v1",
);
assert.equal(
  failureInjection.coverage_model,
  "DERIVE_EVERY_CUT_FROM_TRANSACTION_BOUNDARIES_MANIFEST",
);
assertDeepEqual(failureInjection.transaction_cut_rule, {
  before_first_statement: "NO_DURABLE_EFFECT",
  after_every_statement_before_next_or_commit:
    "INJECT_TRANSACTION_ABORT_AND_REQUIRE_COMPLETE_ROLLBACK",
  after_commit:
    "RETRY_MUST_OBSERVE_COMPLETE_DURABLE_TRANSACTION_WITH_NO_PARTIAL_EFFECT",
  after_commit_before_response:
    "UNKNOWN_RESULT_RETRY_MUST_RECONCILE_USING_STABLE_OPERATION_OR_IDEMPOTENCY_ID_AND_CREATE_NO_DUPLICATE_EFFECT",
  commit_response_flags: [
    "commit_before_response",
    "commit_before_credential_response",
    "commit_before_challenge_response",
    "commit_before_rejection_response",
  ],
  statement_order_source: "transaction-boundaries-v1.json",
  conditional_statement_policy: "EXERCISE_BOTH_ABSENT_AND_PRESENT_PATHS",
});
assertDeepEqual(failureInjection.external_cut_rule, {
  publish_retry_identity: ["event_id", "payload_digest"],
  ack_loss:
    "REDELIVERY_MUST_HIT_INBOX_AND_CREATE_NO_DUPLICATE_BUSINESS_EFFECT",
  event_retention: "NEVER_DELETE_IN_PHASE_1",
});
assertDeepEqual(
  failureInjection.workflow_requirements.map(({ workflow }) => workflow).sort(),
  transactions.workflows.map(({ name }) => name).sort(),
  "failure injection must cover every transaction workflow",
);
const failureCutEvidence = [];
function derivedTransactionGroups(workflow) {
  if (workflow.must_not_persist === true) return [];
  if (Array.isArray(workflow.single_postgresql_transaction)) {
    return [
      [
        "single_postgresql_transaction",
        ...(Array.isArray(workflow.conditional_same_transaction)
          ? ["conditional_same_transaction"]
          : []),
      ],
    ];
  }
  const groups = [];
  for (const field of [
    "claim_postgresql_transaction",
    "mark_postgresql_transaction",
  ]) {
    if (Array.isArray(workflow[field])) groups.push([field]);
  }
  return groups;
}

function derivedExternalBoundaries(workflow) {
  if (workflow.ack_after_commit === true) {
    return [
      "AFTER_POSTGRESQL_COMMIT_BEFORE_NATS_ACK",
      "AFTER_NATS_ACK",
    ];
  }
  if (Array.isArray(workflow.broker_phase)) {
    assertDeepEqual(workflow.broker_phase, [
      "publish_exact_event_id_and_payload_digest",
      "receive_broker_ack",
    ]);
    assert(Array.isArray(workflow.claim_postgresql_transaction));
    assert(Array.isArray(workflow.mark_postgresql_transaction));
    return [
      "AFTER_CLAIM_COMMIT_BEFORE_PUBLISH",
      "AFTER_PUBLISH_BEFORE_BROKER_ACK",
      "AFTER_BROKER_ACK_BEFORE_MARK_TRANSACTION",
      "AFTER_MARK_COMMIT",
    ];
  }
  return [];
}

for (const workflow of transactions.workflows) {
  const requirement = failureInjection.workflow_requirements.find(
    ({ workflow: name }) => name === workflow.name,
  );
  assert(requirement, `missing failure requirement for ${workflow.name}`);
  assertDeepEqual(
    requirement.transaction_groups,
    derivedTransactionGroups(workflow),
    `${workflow.name} failure transaction groups were not derived from the transaction manifest`,
  );
  const expectedExternalBoundaries = derivedExternalBoundaries(workflow);
  assertDeepEqual(
    requirement.external_boundaries ?? [],
    expectedExternalBoundaries,
    `${workflow.name} external cuts were not derived from publish/ack behavior`,
  );
  if (requirement.must_not_persist === true) {
    assert.equal(workflow.must_not_persist, true, workflow.name);
    assertDeepEqual(requirement.transaction_groups, [], workflow.name);
    assert(
      Array.isArray(workflow.response_steps) && workflow.response_steps.length > 0,
      `${workflow.name} must define a nonpersistent response`,
    );
    failureCutEvidence.push(`${workflow.name}:NO_PERSISTENCE_PROVEN`);
    continue;
  }
  for (const [groupIndex, fields] of requirement.transaction_groups.entries()) {
    assert(Array.isArray(fields) && fields.length > 0, workflow.name);
    const statements = fields.flatMap((field) => {
      assert(
        Array.isArray(workflow[field]) && workflow[field].length > 0,
        `${workflow.name}.${field} has no statements`,
      );
      return workflow[field].map((statement) => ({ field, statement }));
    });
    const transactionName = `TX${groupIndex + 1}`;
    failureCutEvidence.push(
      `${workflow.name}:${transactionName}:BEFORE_FIRST_STATEMENT`,
    );
    for (const [statementIndex, { field, statement }] of statements.entries()) {
      assert.equal(typeof statement, "string", `${workflow.name}.${field}`);
      failureCutEvidence.push(
        `${workflow.name}:${transactionName}:AFTER_STATEMENT_${statementIndex + 1}:${field}:${statement}`,
      );
    }
    failureCutEvidence.push(`${workflow.name}:${transactionName}:AFTER_COMMIT`);
  }
  if (
    failureInjection.transaction_cut_rule.commit_response_flags.some(
      (flag) => workflow[flag] === true,
    )
  ) {
    failureCutEvidence.push(
      `${workflow.name}:RESPONSE:AFTER_COMMIT_BEFORE_RESPONSE:UNKNOWN_REQUIRES_RECONCILIATION`,
    );
  }
  for (const boundary of expectedExternalBoundaries) {
    failureCutEvidence.push(`${workflow.name}:EXTERNAL:${boundary}`);
  }
}
assert.equal(
  new Set(failureCutEvidence).size,
  failureCutEvidence.length,
  "derived failure cut evidence contains duplicates",
);
const ordinaryEnrollment = transactions.workflows.find(
  ({ name }) => name === "ordinary_device_enrollment",
);
assert(
  ordinaryEnrollment.must_not_write.includes("prior_device_revocation"),
);
const replacementEnrollment = transactions.workflows.find(
  ({ name }) => name === "replacement_device_enrollment",
);
assert.equal(replacementEnrollment.revocation_before_new_session, true);

assert.equal(
  migration.immediate_predecessor_definition,
  "DIRECTLY_PRIOR_NUMBERED_SCHEMA_REVISION",
);
assertDeepEqual(migration.later_revision_applies_from, [
  "EMPTY_DATABASE",
  "IMMEDIATE_PREDECESSOR",
]);
assert.equal(migration.destructive_cleanup.allowed_in_phase_1, false);
assert.equal(
  migration.outbox_business_event_deletion,
  "FORBIDDEN_IN_PHASE_1",
);
assertDeepEqual(
  recoveryPolicy.verification_gates,
  [
    "constraints",
    "aggregate_versions",
    "outbox_continuity",
    "audit_linkage",
    "fixture_checksums",
  ],
);
assert(
  Object.values(recoveryPolicy.restore_capabilities).every(
    (capability) => capability === false,
  ),
  "recovery restore must contain no trading capability",
);
assertDeepEqual(recoveryPolicy.fail_closed_evidence_contract, {
  required_result: "FAIL_CLOSED",
  minimum_failed_verification_gates: 1,
  pass_requires_every_verification_gate: "PASS",
  observed_rto_must_be_absent: true,
  restore_stop_event_required: true,
  failure_reason_code_required: true,
  restore_start_and_terminal_event_ids_must_differ: true,
  required_components: ["postgresql", "recovery-tool"],
});

assert(
  failureCutEvidence.some((cut) => cut.includes("AFTER_STATEMENT_")),
  "no derived statement cut",
);
assert(
  failureCutEvidence.some((cut) => cut.includes("AFTER_COMMIT")),
  "no derived commit cut",
);
assert(
  failureCutEvidence.some((cut) =>
    cut.includes("UNKNOWN_REQUIRES_RECONCILIATION"),
  ),
  "no derived post-commit response-loss reconciliation cut",
);
assert(
  failureCutEvidence.some((cut) => cut.includes("PUBLISH")),
  "no publish cut",
);
assert(
  failureCutEvidence.some((cut) => cut.includes("ACK")),
  "no acknowledgement cut",
);
assert.equal(
  failureInjection.coverage_requirements.every_postgresql_statement_boundary,
  true,
);
assert.equal(
  failureInjection.coverage_requirements.every_commit_boundary,
  true,
);
assert.equal(
  failureInjection.coverage_requirements.every_publish_boundary,
  true,
);
assert.equal(
  failureInjection.coverage_requirements.every_ack_boundary,
  true,
);
assert.equal(
  failureInjection.coverage_requirements.every_transaction_manifest_workflow,
  true,
);
assert.equal(
  failureInjection.coverage_requirements.every_postgresql_transaction_segment,
  true,
);
assert.equal(
  failureInjection.coverage_requirements.zero_persistence_workflows_prove_no_write,
  true,
);

assert.equal(stateMachines.machines.challenge.ttl_seconds, 120);
assert.equal(
  stateMachines.machines.refresh.family_deadline_seconds,
  2_592_000,
);
assert.equal(
  stateMachines.machines.refresh.individual_ttl_seconds,
  604_800,
);
assert.equal(
  stateMachines.machines.websocket_reauthorization.deadline_equality_effect,
  "CLOSE_4401",
);
assert.equal(
  stateMachines.machines.wal_archive_incident.age_seconds_60,
  "HEALTHY",
);
assert.equal(
  stateMachines.machines.wal_archive_incident.age_seconds_61,
  "OPEN_INCIDENT",
);

const mcpManifest = await readJson(
  path.join(contractsDirectory, "mcp-tools-v1.json"),
);
const forbiddenModelAuthority = new Set([
  "user_id",
  "account_id",
  "trading_account_id",
  "session_id",
  "device_id",
  "confirmation_id",
  "confirmation_hash",
  "device_signature",
  "device_verification",
  "token_digest",
]);
for (const tool of mcpManifest.tools) {
  const inputProperties = Object.keys(tool.input_schema.properties ?? {});
  for (const property of inputProperties) {
    assert(
      !forbiddenModelAuthority.has(property),
      `${tool.name} input carries server authority ${property}`,
    );
  }
  assert.equal(tool.input_schema.additionalProperties, false);
  assertDeepEqual(tool.server_injected_scope, [
    "user_id",
    "account_id",
    "session_id",
  ]);
}

for (const [relativePath, expectedHash] of Object.entries(
  phase0Manifest.immutable_files,
)) {
  const content = await readFile(path.join(contractsDirectory, relativePath));
  assert.equal(
    sha256(content),
    expectedHash,
    `Phase 0 file changed: contracts/${relativePath}`,
  );
}

const baseTreeFiles = execFileSync(
  "git",
  [
    "-C",
    repositoryDirectory,
    "ls-tree",
    "-r",
    "--name-only",
    phase0Manifest.base_commit,
    "contracts",
  ],
  { encoding: "utf8" },
)
  .trim()
  .split("\n")
  .filter(Boolean)
  .map((entry) => entry.replace(/^contracts\//, ""))
  .filter((entry) => entry !== "package.json")
  .sort();
assertDeepEqual(
  Object.keys(phase0Manifest.immutable_files).sort(),
  baseTreeFiles,
  "Phase 0 immutable manifest coverage drifted",
);

const packageJson = await readJson(path.join(contractsDirectory, "package.json"));
assertDeepEqual(packageJson.devDependencies, {
  "@apidevtools/swagger-parser": "12.1.0",
  ajv: "8.20.0",
  "ajv-formats": "3.0.1",
  protobufjs: "8.7.1",
  yaml: "2.9.0",
});
assertDeepEqual(packageJson.scripts, {
  test: "npm run test:contracts && npm run test:platform && npm run test:history-secrets",
  "test:contracts": "node scripts/verify.mjs",
  "test:platform": "node scripts/verify-platform.mjs",
  "test:history-secrets":
    "git -C .. log -p --all -- . | node scripts/scan-stdin-secrets.mjs",
});

const changedPaths = execFileSync(
  "git",
  [
    "-C",
    repositoryDirectory,
    "diff",
    "--name-only",
    phase0Manifest.base_commit,
    "--",
  ],
  { encoding: "utf8" },
)
  .trim()
  .split("\n")
  .filter(Boolean);
const allowedPath = (entry) =>
  entry === "contracts/package.json" ||
  entry === "contracts/scripts/verify-platform.mjs" ||
  entry.startsWith("contracts/platform/") ||
  entry.startsWith("contracts/fixtures/platform/");
for (const changedPath of changedPaths) {
  assert(allowedPath(changedPath), `P1-001 changed forbidden path ${changedPath}`);
}

const platformFiles = await readdir(platformDirectory, {
  recursive: true,
  withFileTypes: true,
});
assert(
  platformFiles.some(
    (entry) => entry.isFile() && entry.name === "platform-v1.schema.json",
  ),
);

const semanticScenarioCount = Object.values(scenarios)
  .filter((value) => Array.isArray(value))
  .reduce((count, value) => count + value.length, 0);
const executableSecurityScenarioCount = Object.values(executableSecurity)
  .filter((value) => Array.isArray(value))
  .reduce((count, value) => count + value.length, 0);

console.log(
  [
    `Platform schemas: ${platformValidators.size + 2}`,
    `Valid fixtures: ${validFixtureSet.cases.length}`,
    `Negative fixtures: ${invalidFixtureSet.cases.length}`,
    `Derived failure cuts: ${failureCutEvidence.length}`,
    `Argon2 vector: ${argonVectorExecution}`,
    `Semantic scenarios: ${semanticScenarioCount}`,
    `Executable security scenario groups: ${executableSecurityScenarioCount}`,
    `Persistence linkage chains: ${linkageChains.chains.length}`,
    "Phase 1 platform contract verification passed.",
  ].join("\n"),
);
