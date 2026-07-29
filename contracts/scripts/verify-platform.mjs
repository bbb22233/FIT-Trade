import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import {
  createHash,
  createHmac,
  createPublicKey,
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
      Object.getPrototypeOf(value) === Object.prototype,
    "JCS accepts only plain JSON objects",
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
    while (/\s/u.test(raw[cursor] ?? "")) cursor += 1;
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
    const value = {};
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

function websocketOutcome(item) {
  if (item.revoked) {
    return "CLOSE_REVOKED_WITHIN_5_SECONDS";
  }
  if (item.event === "APPLICATION_FRAME_WHILE_REAUTH_PENDING") {
    return "REJECT_APPLICATION_FRAME";
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
  return { failures: 0, nextAllowedMs: 0, lockedUntilMs: 0 };
}

function clearExpiredThrottleState(state, nowMs) {
  if (state.lockedUntilMs > 0 && nowMs >= state.lockedUntilMs) {
    state.failures = 0;
    state.nextAllowedMs = 0;
    state.lockedUntilMs = 0;
  }
}

function runThrottleTimeline(timeline, throttle) {
  const source = newThrottleDimension();
  const accounts = new Map();
  for (const event of timeline.events) {
    const nowMs = event.at_seconds * 1000;
    const account =
      event.user_id === null
        ? null
        : (accounts.get(event.user_id) ?? newThrottleDimension());
    if (account !== null) accounts.set(event.user_id, account);
    clearExpiredThrottleState(source, nowMs);
    if (account !== null) clearExpiredThrottleState(account, nowMs);
    const dimensions = [source, ...(account === null ? [] : [account])];
    let decision;
    let appliedDelaySeconds = 0;
    if (dimensions.some((state) => nowMs < state.lockedUntilMs)) {
      decision = "DENY_LOCK";
    } else if (dimensions.some((state) => nowMs < state.nextAllowedMs)) {
      decision = "DENY_DELAY";
    } else if (event.password_valid) {
      account.failures = 0;
      account.nextAllowedMs = 0;
      account.lockedUntilMs = 0;
      decision = "PASSWORD_ACCEPTED";
    } else {
      for (const state of dimensions) {
        state.failures += 1;
        if (state.failures >= throttle.lock_on_failure) {
          state.lockedUntilMs = nowMs + throttle.lock_seconds * 1000;
          state.nextAllowedMs = 0;
        } else {
          appliedDelaySeconds =
            throttle.failure_delays_seconds[state.failures - 1];
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
  keyword: "x-fit-time-window",
  schemaType: "object",
  type: "object",
  errors: false,
  validate: (rule, data) => {
    const start = Date.parse(data[rule.start]);
    const end = Date.parse(data[rule.end]);
    return (
      Number.isFinite(start) &&
      Number.isFinite(end) &&
      end - start === rule.seconds * 1000 &&
      (!Object.hasOwn(data, "revoked_at") ||
        Date.parse(data.revoked_at) >= start)
    );
  },
});
ajv.addKeyword({
  keyword: "x-fit-refresh-window",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    const issuedAt = Date.parse(data.issued_at);
    const familyDeadline = Date.parse(data.family_deadline);
    const expiresAt = Date.parse(data.expires_at);
    const revokedAt = Object.hasOwn(data, "revoked_at")
      ? Date.parse(data.revoked_at)
      : null;
    return (
      Number.isFinite(issuedAt) &&
      Number.isFinite(familyDeadline) &&
      Number.isFinite(expiresAt) &&
      familyDeadline > issuedAt &&
      expiresAt === Math.min(issuedAt + 604_800_000, familyDeadline) &&
      (revokedAt === null ||
        (Number.isFinite(revokedAt) &&
          revokedAt >= issuedAt &&
          revokedAt <= familyDeadline))
    );
  },
});
ajv.addKeyword({
  keyword: "x-fit-websocket-deadline",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    const issuedAt = Date.parse(data.issued_at);
    const deadline = Date.parse(data.deadline);
    const accessExpiry = Date.parse(data.current_access_expires_at);
    return (
      Number.isFinite(issuedAt) &&
      Number.isFinite(deadline) &&
      Number.isFinite(accessExpiry) &&
      issuedAt < accessExpiry &&
      deadline === Math.min(issuedAt + 60_000, accessExpiry)
    );
  },
});
ajv.addKeyword({
  keyword: "x-fit-payload-integrity",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) =>
    data.payload?.schema_version === data.payload_schema_version &&
    sha256(jcsCanonicalize(data.payload)) === data.payload_digest,
});
ajv.addKeyword({
  keyword: "x-fit-recovery-consistency",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    const committedAt = Date.parse(data.last_committed?.committed_at);
    const recoveredAt = Date.parse(data.last_recovered?.committed_at);
    const injectedAt = Date.parse(data.failure?.injected_at);
    const invokedAt = Date.parse(data.restore?.invoked_at);
    const completedAt = Date.parse(data.restore?.verification_completed_at);
    return (
      [
        committedAt,
        recoveredAt,
        injectedAt,
        invokedAt,
        completedAt,
      ].every(Number.isFinite) &&
      data.last_committed.sequence === 6000 &&
      data.last_recovered.sequence <= data.last_committed.sequence &&
      recoveredAt <= committedAt &&
      committedAt === injectedAt &&
      injectedAt <= invokedAt &&
      invokedAt <= completedAt &&
      data.observed_rpo.sequence_gap ===
        data.last_committed.sequence - data.last_recovered.sequence &&
      data.observed_rpo.time_gap_ms === committedAt - recoveredAt &&
      data.observed_rto_ms === completedAt - invokedAt
    );
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
for (const forbiddenField of [
  "authorization",
  "ownership_id",
  "refresh_family_id",
  "identity_origin",
  "role",
  "actor",
  "scope",
  "issued_at",
  "revoked_at",
  "signature_verified",
]) {
  const mutation = clone(validUntrustedMutation);
  mutation.body = {
    nested: [{ deeper: { [forbiddenField]: "client-forged-authority" } }],
  };
  assert(
    !validateUntrustedMutation(mutation),
    `untrusted recursive input accepted server authority ${forbiddenField}`,
  );
}

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
    isoMilliseconds(validRefreshRecord.issued_at),
  security.tokens.refresh_family_max_lifetime_seconds * 1000,
  "initial refresh family deadline drifted",
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
for (const scenario of executableSecurity.synthetic_enrollment_cases) {
  assert(
    validatorFor("EnrollmentChallenge")(scenario.wire),
    `synthetic challenge wire is invalid: ${scenario.name}`,
  );
  assert.equal(scenario.identifier_resolved, false, scenario.name);
  assert.equal(scenario.server_state_exists, false, scenario.name);
  assert.equal(scenario.persisted, false, scenario.name);
  assert.equal(scenario.completion_attempted, true, scenario.name);
  assert.equal(scenario.devices_after, scenario.devices_before, scenario.name);
  assert.equal(scenario.sessions_after, scenario.sessions_before, scenario.name);
  assert.equal(
    scenario.refresh_families_after,
    scenario.refresh_families_before,
    scenario.name,
  );
  assert.equal(
    challengeOutcome({
      synthetic: true,
      already_attempted: false,
      now: scenario.wire.issued_at,
      expires_at: scenario.wire.expires_at,
      proof_valid: true,
    }),
    scenario.expected,
    scenario.name,
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
  assert.equal(matches ? "ACCEPT" : "REJECT", scenario.expected, scenario.name);
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

const validEventEnvelope = validFixtureSet.cases.find(
  ({ schema }) => schema === "EventEnvelope",
).value;
for (const mutate of [
  (value) => {
    value.payload.decision = "REJECT";
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
  "fit.platform.v1.auth-security.login-account-locked",
  "fit.platform.v1.auth-security.login-source-locked",
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
      schema_version: "fit.platform.persistence-link.v1",
      record_type: durable.recordType,
      record_id: durable.recordId,
      causation_id: chain.causation_id,
      correlation_id: chain.correlation_id,
      scope: chain.scope,
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
    outboxRecords.map(({ event }) => event.payload.record_id),
    durableRecords.map(({ recordId }) => recordId),
    `${chain.name} could not rebuild durable links from Outbox payloads`,
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
  if (chain.trigger_context.identifier_resolved === false) {
    assert(!Object.hasOwn(chain.scope, "user_id"), chain.name);
    assert(!Object.hasOwn(chain.scope, "trading_account_id"), chain.name);
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
            ...(binding.event_kind === "LOGIN_ACCOUNT_LOCKED"
              ? { user_id: "10000000-0000-4000-8000-000000000001" }
              : {}),
          }
        : { type: "SYSTEM" };
  const payload = {
    schema_version: "fit.platform.synthetic-event.v1",
    subject: binding.subject,
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
    causation_id: "17000000-0000-4000-8000-000000000001",
    correlation_id: "18000000-0000-4000-8000-000000000001",
    occurred_at: "2026-07-29T10:00:01Z",
    payload_schema_version: "fit.platform.synthetic-event.v1",
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
  if (binding.event_kind === "LOGIN_ACCOUNT_LOCKED") {
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
assert.equal(
  failureInjection.coverage_model,
  "DERIVE_EVERY_CUT_FROM_TRANSACTION_BOUNDARIES_MANIFEST",
);
assertDeepEqual(
  failureInjection.workflow_requirements.map(({ workflow }) => workflow).sort(),
  transactions.workflows.map(({ name }) => name).sort(),
  "failure injection must cover every transaction workflow",
);
const failureCutEvidence = [];
for (const workflow of transactions.workflows) {
  const requirement = failureInjection.workflow_requirements.find(
    ({ workflow: name }) => name === workflow.name,
  );
  assert(requirement, `missing failure requirement for ${workflow.name}`);
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
  for (const boundary of requirement.external_boundaries ?? []) {
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

assert(
  failureCutEvidence.some((cut) => cut.includes("AFTER_STATEMENT_")),
  "no derived statement cut",
);
assert(
  failureCutEvidence.some((cut) => cut.includes("AFTER_COMMIT")),
  "no derived commit cut",
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
