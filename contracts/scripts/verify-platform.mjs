import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import {
  createCipheriv,
  createDecipheriv,
  createHash,
  createHmac,
  generateKeyPairSync,
  createPublicKey,
  randomBytes,
  sign as signMessage,
  timingSafeEqual,
  verify as verifySignature,
} from "node:crypto";
import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const nodeCrypto = await import("node:crypto");
const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const contractsDirectory = path.resolve(scriptDirectory, "..");
const repositoryDirectory = path.resolve(contractsDirectory, "..");
const platformDirectory = path.join(contractsDirectory, "platform");
const fixtureDirectory = path.join(contractsDirectory, "fixtures", "platform");

const readJson = async (filePath) =>
  JSON.parse(await readFile(filePath, "utf8"));

const sha256 = (value) => createHash("sha256").update(value).digest("hex");

const clone = (value) => JSON.parse(JSON.stringify(value));
const responseCacheTtlMilliseconds = 120_000;
const responseCacheKeyId = "fit-platform-response-cache-runtime-v1";
const testOnlyDeploymentKeyMaterial = {
  [responseCacheKeyId]:
    "746573742d6f6e6c792d6465706c6f796d656e742d6b65792d6d617465726961",
};
const acceptedPhase0BaseCommit =
  "5f168cd4ebfa1ee7f930425fa601c608257ea360";
const acceptedPhase0ManifestVersion = "fit.platform.phase0-file-manifest.v1";
const acceptedPackageBaseSha256 =
  "5c14e293371607df5e719129531cfb1bb2c689cda2825c6ac8617ea44d576744";

function cacheTimeMilliseconds(value) {
  if (typeof value === "number") {
    assert(Number.isSafeInteger(value) && value >= 0, "invalid cache time");
    return value;
  }
  return isoMilliseconds(value);
}

function loadDeploymentKeyring(keyMaterial) {
  const keyring = new Map();
  for (const [keyId, keyHex] of Object.entries(keyMaterial)) {
    assert.match(keyId, /^[a-z0-9][a-z0-9._-]*$/u);
    assert.match(keyHex, /^[a-f0-9]{64}$/u);
    keyring.set(keyId, Buffer.from(keyHex, "hex"));
  }
  return keyring;
}

const primaryResponseCacheKeyring = loadDeploymentKeyring(
  testOnlyDeploymentKeyMaterial,
);
const replicaResponseCacheKeyring = loadDeploymentKeyring(
  testOnlyDeploymentKeyMaterial,
);
const restartedResponseCacheKeyring = loadDeploymentKeyring(
  testOnlyDeploymentKeyMaterial,
);

function canonicalUuid(value) {
  assert.equal(typeof value, "string");
  assert.match(
    value,
    /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/iu,
  );
  return value.toLowerCase();
}

function responseCacheContext({
  userId,
  tradingAccountId,
  routeTemplate,
  requestKey,
  requestDigest,
}) {
  const context = {
    user_id: canonicalUuid(userId),
    trading_account_id: canonicalUuid(tradingAccountId),
    route_template: routeTemplate,
    idempotency_key: canonicalUuid(requestKey),
    request_digest: requestDigest,
  };
  assert.match(context.route_template, /^\//u);
  assert.match(
    context.idempotency_key,
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/iu,
  );
  assert.match(context.request_digest, /^[a-f0-9]{64}$/u);
  return context;
}

function responseLedgerKey(context) {
  return jcsCanonicalize({
    user_id: context.user_id,
    trading_account_id: context.trading_account_id,
    route_template: context.route_template,
    idempotency_key: context.idempotency_key,
  });
}

function cachedResponseAad(cache) {
  return jcsCanonicalize({
    schema_version: "fit.platform.response-cache-aad.v1",
    key_id: cache.key_id,
    user_id: cache.user_id,
    trading_account_id: cache.trading_account_id,
    route_template: cache.route_template,
    idempotency_key: cache.idempotency_key,
    request_digest: cache.request_digest,
    created_at_ms: cache.created_at_ms,
    expires_at_ms: cache.expires_at_ms,
  });
}

function assertResponseCacheContext(cache, context) {
  assertDeepEqual(
    {
      user_id: cache.user_id,
      trading_account_id: cache.trading_account_id,
      route_template: cache.route_template,
      idempotency_key: cache.idempotency_key,
      request_digest: cache.request_digest,
    },
    context,
    "response cache scope or digest binding mismatch",
  );
}

function sealCachedResponse(
  response,
  context,
  createdAt,
  keyring,
) {
  assert(keyring instanceof Map, "response-cache keyring must be injected");
  const createdAtMs = cacheTimeMilliseconds(createdAt);
  const cache = {
    algorithm: "AES-256-GCM",
    key_id: responseCacheKeyId,
    ...context,
    created_at_ms: createdAtMs,
    expires_at_ms: createdAtMs + responseCacheTtlMilliseconds,
  };
  const key = keyring.get(cache.key_id);
  assert.equal(key?.length, 32, "response-cache key unavailable");
  const nonce = randomBytes(12);
  const cipher = createCipheriv("aes-256-gcm", key, nonce);
  cipher.setAAD(Buffer.from(cachedResponseAad(cache), "utf8"));
  const ciphertext = Buffer.concat([
    cipher.update(jcsCanonicalize(response), "utf8"),
    cipher.final(),
  ]);
  const authenticationTag = cipher.getAuthTag();
  return {
    ...cache,
    nonce: nonce.toString("base64url"),
    ciphertext: ciphertext.toString("base64url"),
    authentication_tag: authenticationTag.toString("base64url"),
  };
}

function openCachedResponse(
  cache,
  context,
  readAt,
  keyring,
) {
  assert(keyring instanceof Map, "response-cache keyring must be injected");
  assert.equal(cache.algorithm, "AES-256-GCM");
  assertResponseCacheContext(cache, context);
  assert(
    cacheTimeMilliseconds(readAt) < cache.expires_at_ms,
    "response cache expired",
  );
  const key = keyring.get(cache.key_id);
  assert.equal(key?.length, 32, "response-cache key unavailable");
  const decipher = createDecipheriv(
    "aes-256-gcm",
    key,
    Buffer.from(cache.nonce, "base64url"),
  );
  decipher.setAAD(Buffer.from(cachedResponseAad(cache), "utf8"));
  decipher.setAuthTag(Buffer.from(cache.authentication_tag, "base64url"));
  return JSON.parse(
    Buffer.concat([
      decipher.update(Buffer.from(cache.ciphertext, "base64url")),
      decipher.final(),
    ]).toString("utf8"),
  );
}

function expireCachedResponse(ledgerEntry, expiredAt) {
  if (ledgerEntry.response_cache !== undefined) {
    delete ledgerEntry.response_cache;
  }
  ledgerEntry.cache_expired_at_ms = cacheTimeMilliseconds(expiredAt);
}

function sweepExpiredResponseCaches(ledger, now) {
  const nowMs = cacheTimeMilliseconds(now);
  for (const entry of ledger.values()) {
    if (
      entry.response_cache !== undefined &&
      nowMs >= entry.response_cache.expires_at_ms
    ) {
      expireCachedResponse(entry, nowMs);
    }
  }
}

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
  const bound = {
    schema_version:
      input.schema_version === "fit.platform.request-envelope.v1"
        ? "fit.platform.request-digest.v1"
        : input.schema_version,
  };
  for (const field of requestDigestBoundFields.slice(1)) {
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

function enrollmentRouteTemplate(purpose, operation) {
  assert(["start", "complete"].includes(operation));
  if (purpose === "ADDITIONAL_DEVICE") {
    return `/v1/auth/device-enrollments/${operation}`;
  }
  assert.equal(purpose, "REPLACEMENT_DEVICE");
  return `/v1/auth/device-replacements/${operation}`;
}

function authenticatedMutationDigest({
  routeTemplate,
  body,
  userId,
  tradingAccountId,
}) {
  return canonicalRequestDigest({
    schema_version: "fit.platform.request-digest.v1",
    method: "POST",
    route_template: routeTemplate,
    path: {},
    query: {},
    body,
    user_id: canonicalUuid(userId),
    trading_account_id: canonicalUuid(tradingAccountId),
  });
}

function durableRecordDigest(record) {
  const preimage = clone(record);
  delete preimage.payload_digest;
  return sha256(jcsCanonicalize(preimage));
}

function parseStrictRequestTarget(rawTarget) {
  assert.equal(typeof rawTarget, "string", "request target must be raw text");
  assert(rawTarget.startsWith("/"), "request target must be origin-form");
  assert(!rawTarget.includes("#"), "request target fragment is forbidden");
  assert(!rawTarget.includes("\\"), "request target backslash is forbidden");
  assert(
    /^[\x21-\x7e]+$/u.test(rawTarget),
    "request target must use visible ASCII",
  );
  const questionIndex = rawTarget.indexOf("?");
  const rawPath =
    questionIndex === -1 ? rawTarget : rawTarget.slice(0, questionIndex);
  const rawQuery =
    questionIndex === -1 ? "" : rawTarget.slice(questionIndex + 1);
  assert(!rawPath.includes("%"), "percent-encoded path bytes are forbidden");
  const segments = rawPath.split("/");
  assert.equal(segments[0], "", "request path must be absolute");
  assert(
    segments.slice(1).every((segment) => segment.length > 0),
    "empty path segment is forbidden",
  );
  assert(
    segments.slice(1).every((segment) => segment !== "." && segment !== ".."),
    "dot path segment is forbidden",
  );
  assert(
    segments.slice(1).every((segment) => /^[A-Za-z0-9._~-]+$/u.test(segment)),
    "request path contains a noncanonical byte",
  );

  const match = rawPath.match(
    /^\/v1\/confirmations\/([0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12})\/consume$/iu,
  );
  assert(match, "request path does not match the frozen confirmation route");

  const query = Object.create(null);
  if (rawQuery.length > 0) {
    for (const pair of rawQuery.split("&")) {
      assert(pair.length > 0, "empty query pair is forbidden");
      const equalsIndex = pair.indexOf("=");
      const rawKey = equalsIndex === -1 ? pair : pair.slice(0, equalsIndex);
      const rawValue = equalsIndex === -1 ? "" : pair.slice(equalsIndex + 1);
      let key;
      let value;
      try {
        key = decodeURIComponent(rawKey);
        value = decodeURIComponent(rawValue);
      } catch {
        assert.fail("invalid query percent encoding");
      }
      assert(key.length > 0, "empty decoded query key is forbidden");
      assert(!Object.hasOwn(query, key), `duplicate decoded query key ${key}`);
      query[key] = value;
    }
  }
  assert.equal(
    Object.keys(query).length,
    0,
    "confirmation route accepts no query parameters",
  );
  return {
    route_template: "/v1/confirmations/{confirmation_id}/consume",
    path: Object.assign(Object.create(null), {
      confirmation_id: canonicalUuid(match[1]),
    }),
    query,
  };
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
    const match = raw
      .slice(cursor)
      .match(
        /^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/u,
      );
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

function syntheticUuid(label) {
  const digest = sha256(label);
  return [
    digest.slice(0, 8),
    digest.slice(8, 12),
    `4${digest.slice(13, 16)}`,
    `8${digest.slice(17, 20)}`,
    digest.slice(20, 32),
  ].join("-");
}

function enrollmentStateFromFixture(fixture) {
  for (const identity of fixture.identity_directory) {
    assert.equal(typeof identity.trading_account_id, "string");
    assert.equal(typeof identity.source_key, "string");
  }
  for (const ownership of fixture.ownership_records) {
    assert(
      validatorFor("TradingAccountOwnership")(ownership),
      `invalid enrollment ownership fixture: ${ajv.errorsText(
        validatorFor("TradingAccountOwnership").errors,
      )}`,
    );
  }
  return {
    identities: new Map(
      fixture.identity_directory.map((identity) => [
        identity.identifier,
        {
          userId: identity.user_id,
          tradingAccountId: identity.trading_account_id,
          sourceKey: identity.source_key,
          passwordArgon2id: clone(identity.password_argon2id),
          testOnlyPasswordSha256: identity.test_only_password_sha256,
        },
      ]),
    ),
    unknownIdentityVerifier: {
      passwordArgon2id: clone(
        fixture.identity_directory[0].password_argon2id,
      ),
      testOnlyPasswordSha256:
        fixture.identity_directory[0].test_only_password_sha256,
    },
    ownerships: new Map(
      fixture.ownership_records.map((ownership) => [
        `${ownership.user_id}:${ownership.trading_account_id}`,
        clone(ownership),
      ]),
    ),
    challenges: new Map(),
    devices: new Map(),
    sessions: new Map(),
    refreshFamilies: new Map(),
    enrollments: new Map(),
    publicKeyOwners: new Map(),
    auditEvents: new Map(),
    notifications: new Map(),
    outboxRecords: new Map(),
    aggregateState: new Map(),
    responseLedger: new Map(),
  };
}

function verifyPasswordAttempt(identity, passwordUtf8) {
  if (
    typeof passwordUtf8 !== "string" ||
    Buffer.byteLength(passwordUtf8, "utf8") > 1024
  ) {
    return false;
  }
  const verifier = identity.passwordArgon2id;
  if (
    verifier?.profile_id !==
      security.password_hashing.production_profile.profile_id ||
    verifier.version !== security.password_hashing.version
  ) {
    return false;
  }
  if (typeof nodeCrypto.argon2Sync !== "function") {
    return (
      Number(process.versions.node.split(".")[0]) < 24 &&
      sha256(Buffer.from(passwordUtf8, "utf8")) ===
        identity.testOnlyPasswordSha256
    );
  }
  try {
    const expected = Buffer.from(verifier.result_hex, "hex");
    const observed = nodeCrypto.argon2Sync("argon2id", {
      message: Buffer.from(passwordUtf8, "utf8"),
      nonce: Buffer.from(verifier.salt_hex, "hex"),
      parallelism: security.password_hashing.production_profile.parallelism,
      tagLength: security.password_hashing.production_profile.result_bytes,
      memory: security.password_hashing.production_profile.minimum_memory_kib,
      passes: security.password_hashing.production_profile.minimum_iterations,
    });
    return (
      expected.length === observed.length && timingSafeEqual(expected, observed)
    );
  } catch {
    return false;
  }
}

function activeOwnership(state, userId, tradingAccountId) {
  const ownership = state.ownerships.get(`${userId}:${tradingAccountId}`);
  return (
    ownership !== undefined &&
    ownership.user_id === userId &&
    ownership.trading_account_id === tradingAccountId &&
    ownership.role === "OWNER" &&
    ownership.status === "ACTIVE"
  );
}

function persistEnrollmentOutbox(
  state,
  recordType,
  record,
  subject,
  aggregateId,
  expectedAggregateVersion,
) {
  const aggregateKey = `ENROLLMENT:${aggregateId}`;
  const aggregate = state.aggregateState.get(aggregateKey) ?? {
    version: 0,
    lastEventId: null,
  };
  assert(
    Number.isSafeInteger(expectedAggregateVersion) &&
      expectedAggregateVersion >= 0,
    "producer must supply a locked expected aggregate version",
  );
  const expectedVersion = expectedAggregateVersion;
  assert.equal(
    aggregate.version,
    expectedVersion,
    "aggregate expected-version compare-and-set conflict",
  );
  const eventId = record.event_id ?? record.notification_id;
  const eventIdentity = {
    event_id: eventId,
    aggregate_type: "ENROLLMENT",
    aggregate_id: aggregateId,
    aggregate_version: aggregate.version + 1,
    ...(aggregate.lastEventId === null
      ? {}
      : { previous_event_id: aggregate.lastEventId }),
  };
  const payload = {
    schema_version: "fit.platform.event-payload.v1",
    subject,
    event_kind: record.kind,
    scope: record.scope,
    event_identity: eventIdentity,
    data: {
      record_type: recordType,
      record_id: record.event_id ?? record.notification_id,
      record,
    },
  };
  const event = {
    schema_version: "fit.platform.event-envelope.v1",
    subject,
    stream: "FIT_PLATFORM_V1",
    event_id: eventId,
    event_kind: record.kind,
    scope: record.scope,
    aggregate_type: "ENROLLMENT",
    aggregate_id: aggregateId,
    aggregate_version: aggregate.version + 1,
    causation_id: record.causation_id,
    correlation_id: record.correlation_id,
    occurred_at: record.occurred_at,
    payload_schema_version: payload.schema_version,
    payload_digest: sha256(jcsCanonicalize(payload)),
    payload,
    ...(eventIdentity.previous_event_id === undefined
      ? {}
      : { previous_event_id: eventIdentity.previous_event_id }),
  };
  const outbox = {
    schema_version: "fit.platform.outbox-record.v1",
    outbox_id: syntheticUuid(`outbox:${eventId}`),
    event,
    created_at: record.occurred_at,
    publication_state: "PENDING",
    retention: "NEVER_DELETE_IN_PHASE_1",
  };
  assert(
    validatorFor("EventEnvelope")(event),
    `generated enrollment EventEnvelope invalid: ${ajv.errorsText(
      validatorFor("EventEnvelope").errors,
    )}`,
  );
  assert(
    validatorFor("OutboxRecord")(outbox),
    `generated enrollment Outbox invalid: ${ajv.errorsText(
      validatorFor("OutboxRecord").errors,
    )}`,
  );
  state.outboxRecords.set(outbox.outbox_id, outbox);
  const current = state.aggregateState.get(aggregateKey) ?? {
    version: 0,
    lastEventId: null,
  };
  assert.equal(
    current.version,
    expectedVersion,
    "aggregate changed before event append commit",
  );
  state.aggregateState.set(aggregateKey, {
    version: event.aggregate_version,
    lastEventId: event.event_id,
  });
  return outbox;
}

function persistEnrollmentAudit(
  state,
  { kind, scope, requestId, correlationId, occurredAt, aggregateId, deviceId },
) {
  const auditEvent = {
    schema_version: "fit.platform.audit-event.v1",
    event_id: syntheticUuid(
      `audit:${kind}:${requestId}:${state.auditEvents.size + 1}`,
    ),
    kind,
    actor: { type: "SERVICE", id: "auth-service" },
    scope,
    trigger: "REQUEST",
    causation_id: requestId,
    correlation_id: correlationId,
    request_id: requestId,
    ...(deviceId === undefined ? {} : { device_id: deviceId }),
    occurred_at: occurredAt,
  };
  auditEvent.payload_digest = durableRecordDigest(auditEvent);
  assert(
    validatorFor("AuditEvent")(auditEvent),
    `generated enrollment AuditEvent invalid: ${ajv.errorsText(
      validatorFor("AuditEvent").errors,
    )}`,
  );
  state.auditEvents.set(auditEvent.event_id, auditEvent);
  const auditSubjects = {
    ENROLLMENT_CHALLENGE_ISSUED:
      "fit.platform.v1.auth-security.enrollment-challenge-issued",
    ENROLLMENT_PROOF_REJECTED:
      "fit.platform.v1.auth-security.enrollment-proof-rejected",
    DEVICE_ENROLLED: "fit.platform.v1.auth-security.device-enrolled",
    DEVICE_REPLACED: "fit.platform.v1.auth-security.device-replaced",
  };
  persistEnrollmentOutbox(
    state,
    "audit",
    auditEvent,
    auditSubjects[kind],
    aggregateId,
    state.aggregateState.get(`ENROLLMENT:${aggregateId}`)?.version ?? 0,
  );
  return auditEvent;
}

function persistEnrollmentNotification(
  state,
  { kind, scope, requestId, correlationId, occurredAt, aggregateId, deviceId },
) {
  const details =
    kind === "DEVICE_REPLACED"
      ? {
          severity: "CRITICAL",
          messageCode: "security.device.replaced",
          subject: "fit.platform.v1.notification.device-replaced",
        }
      : {
          severity: "INFO",
          messageCode: "security.device.enrolled",
          subject: "fit.platform.v1.notification.device-enrolled",
        };
  const notification = {
    schema_version: "fit.platform.notification.v1",
    notification_id: syntheticUuid(
      `notification:${kind}:${requestId}:${state.notifications.size + 1}`,
    ),
    kind,
    severity: details.severity,
    scope,
    message_code: details.messageCode,
    message_args: { device_id: deviceId },
    causation_id: requestId,
    correlation_id: correlationId,
    occurred_at: occurredAt,
  };
  notification.payload_digest = durableRecordDigest(notification);
  assert(
    validatorFor("Notification")(notification),
    `generated enrollment Notification invalid: ${ajv.errorsText(
      validatorFor("Notification").errors,
    )}`,
  );
  state.notifications.set(notification.notification_id, notification);
  persistEnrollmentOutbox(
    state,
    "notification",
    notification,
    details.subject,
    aggregateId,
    state.aggregateState.get(`ENROLLMENT:${aggregateId}`)?.version ?? 0,
  );
  return notification;
}

function replaceRuntimeState(target, source) {
  for (const key of Object.keys(target)) {
    delete target[key];
  }
  Object.assign(target, source);
}

function enrollmentServerContextFromWire(wire) {
  return {
    received_at: wire.issued_at,
    subject_handle: wire.subject_handle,
    nonce: wire.nonce,
  };
}

function buildEnrollmentChallenge(input, serverContext) {
  assertDeepEqual(
    Object.keys(serverContext).sort(),
    ["nonce", "received_at", "subject_handle"],
    "enrollment issuance accepts only server clock and randomness",
  );
  const issuedAtMs = isoMilliseconds(serverContext.received_at);
  const challenge = {
    schema_version: "fit.platform.enrollment-challenge.v1",
    domain: "FIT_TRADE_DEVICE_ENROLLMENT_V1",
    purpose: input.purpose,
    subject_handle: serverContext.subject_handle,
    candidate_public_key_fingerprint:
      input.candidate_public_key_fingerprint,
    nonce: serverContext.nonce,
    issued_at: serverContext.received_at,
    expires_at: new Date(issuedAtMs + 120_000)
      .toISOString()
      .replace(".000Z", "Z"),
    single_use: true,
  };
  assert(
    validatorFor("EnrollmentChallenge")(challenge),
    `server-generated enrollment Challenge invalid: ${ajv.errorsText(
      validatorFor("EnrollmentChallenge").errors,
    )}`,
  );
  return challenge;
}

function startEnrollment(
  state,
  input,
  serverContext,
  requestKey,
  keyring,
) {
  if (
    !validatorFor("EnrollmentStartInput")(input) ||
    typeof requestKey !== "string" ||
    !(keyring instanceof Map)
  ) {
    return { wire: null, persisted: false, outcome: "REJECT_MALFORMED" };
  }
  try {
    canonicalUuid(requestKey);
  } catch {
    return { wire: null, persisted: false, outcome: "REJECT_MALFORMED" };
  }
  const identity = state.identities.get(input.identifier);
  const passwordValid = verifyPasswordAttempt(
    identity ?? state.unknownIdentityVerifier,
    input.password_utf8,
  );
  if (
    identity === undefined ||
    !passwordValid
  ) {
    let wire;
    try {
      wire = buildEnrollmentChallenge(input, serverContext);
    } catch {
      return { wire: null, persisted: false, outcome: "REJECT_MALFORMED" };
    }
    return { wire, persisted: false };
  }
  const routeTemplate = enrollmentRouteTemplate(input.purpose, "start");
  const canonicalDigest = authenticatedMutationDigest({
    routeTemplate,
    body: input,
    userId: identity.userId,
    tradingAccountId: identity.tradingAccountId,
  });
  const cacheContext = responseCacheContext({
    userId: identity.userId,
    tradingAccountId: identity.tradingAccountId,
    routeTemplate,
    requestKey,
    requestDigest: canonicalDigest,
  });
  const ledgerKey = responseLedgerKey(cacheContext);
  const prior = state.responseLedger.get(ledgerKey);
  if (prior !== undefined) {
    if (prior.requestDigest !== canonicalDigest) {
      return {
        wire: null,
        persisted: true,
        outcome: "IDEMPOTENCY_CONFLICT",
      };
    }
    if (
      prior.response_cache === undefined ||
      cacheTimeMilliseconds(serverContext.received_at) >=
        prior.response_cache.expires_at_ms
    ) {
      expireCachedResponse(prior, serverContext.received_at);
      return {
        wire: null,
        persisted: true,
        outcome: "RECONCILIATION_REQUIRED_CACHE_EXPIRED",
      };
    }
    const recorded = openCachedResponse(
      prior.response_cache,
      cacheContext,
      serverContext.received_at,
      keyring,
    );
    return recorded;
  }
  if (
    !activeOwnership(state, identity.userId, identity.tradingAccountId)
  ) {
    return {
      wire: null,
      persisted: false,
      outcome: "REJECT_INACTIVE_OWNERSHIP",
    };
  }
  let wire;
  try {
    wire = buildEnrollmentChallenge(input, serverContext);
  } catch {
    return { wire: null, persisted: false, outcome: "REJECT_MALFORMED" };
  }
  const staged = structuredClone(state);
  const requestId = syntheticUuid(`enrollment-start:${wire.subject_handle}`);
  const correlationId = syntheticUuid(
    `enrollment-correlation:${wire.subject_handle}`,
  );
  const challengeState = {
    schema_version: "fit.platform.enrollment-challenge-state.v1",
    subject_handle_digest: sha256(wire.subject_handle),
    user_id: identity.userId,
    trading_account_id: identity.tradingAccountId,
    source_key: identity.sourceKey,
    request_id: requestId,
    challenge: clone(wire),
    status: "ACTIVE",
    created_at: wire.issued_at,
  };
  assert(
    validatorFor("EnrollmentChallengeState")(challengeState),
    `generated EnrollmentChallengeState invalid: ${ajv.errorsText(
      validatorFor("EnrollmentChallengeState").errors,
    )}`,
  );
  staged.challenges.set(wire.subject_handle, challengeState);
  persistEnrollmentAudit(staged, {
    kind: "ENROLLMENT_CHALLENGE_ISSUED",
    scope: {
      type: "AUTH_SECURITY",
      source_key: identity.sourceKey,
      user_id: identity.userId,
    },
    requestId,
    correlationId,
    occurredAt: wire.issued_at,
    aggregateId: syntheticUuid(`challenge:${wire.subject_handle}`),
  });
  const response = {
    outcome: "CHALLENGE_CREATED",
    replay_classification: "RETURN_RECORDED_CHALLENGE",
    wire: clone(wire),
    persisted: true,
  };
  staged.responseLedger.set(ledgerKey, {
    requestDigest: canonicalDigest,
    response_cache: sealCachedResponse(
      response,
      cacheContext,
      serverContext.received_at,
      keyring,
    ),
  });
  replaceRuntimeState(state, staged);
  return response;
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

function recordEnrollmentResponse(
  state,
  cacheContext,
  outcome,
  createdAt,
  keyring,
  responseFields = {},
) {
  const response = {
    replay_classification: "RETURN_RECORDED_RESULT",
    outcome,
    ...responseFields,
  };
  state.responseLedger.set(responseLedgerKey(cacheContext), {
    requestDigest: cacheContext.request_digest,
    response_cache: sealCachedResponse(
      response,
      cacheContext,
      createdAt,
      keyring,
    ),
  });
  return response;
}

function completeEnrollment(
  state,
  completion,
  receivedAt,
  requestKey,
  keyring,
) {
  if (
    !validatorFor("EnrollmentCompletionInput")(completion) ||
    typeof requestKey !== "string" ||
    !(keyring instanceof Map)
  ) {
    return { outcome: "REJECT_MALFORMED" };
  }
  try {
    canonicalUuid(requestKey);
    isoMilliseconds(receivedAt);
  } catch {
    return { outcome: "REJECT_MALFORMED" };
  }
  const challengeState = state.challenges.get(completion.subject_handle);
  if (challengeState === undefined) {
    return { outcome: "REJECT_NO_SERVER_STATE" };
  }
  const routeTemplate = enrollmentRouteTemplate(
    challengeState.challenge.purpose,
    "complete",
  );
  const canonicalDigest = authenticatedMutationDigest({
    routeTemplate,
    body: completion,
    userId: challengeState.user_id,
    tradingAccountId: challengeState.trading_account_id,
  });
  const staged = structuredClone(state);
  const result = completeEnrollmentTransaction(
    staged,
    completion,
    receivedAt,
    requestKey,
    canonicalDigest,
    routeTemplate,
    keyring,
  );
  replaceRuntimeState(state, staged);
  return result;
}

function completeEnrollmentTransaction(
  state,
  completion,
  receivedAt,
  requestKey,
  requestDigest,
  routeTemplate,
  keyring,
) {
  const challengeState = state.challenges.get(completion.subject_handle);
  if (challengeState === undefined) return { outcome: "REJECT_NO_SERVER_STATE" };
  const cacheContext = responseCacheContext({
    userId: challengeState.user_id,
    tradingAccountId: challengeState.trading_account_id,
    routeTemplate,
    requestKey,
    requestDigest,
  });
  const ledgerKey = responseLedgerKey(cacheContext);
  const priorResult = state.responseLedger.get(ledgerKey);
  if (priorResult !== undefined) {
    if (priorResult.requestDigest !== requestDigest) {
      return { outcome: "IDEMPOTENCY_CONFLICT" };
    }
    if (
      priorResult.response_cache === undefined ||
      cacheTimeMilliseconds(receivedAt) >=
      priorResult.response_cache.expires_at_ms
    ) {
      expireCachedResponse(priorResult, receivedAt);
      return { outcome: "RECONCILIATION_REQUIRED_CACHE_EXPIRED" };
    }
    return openCachedResponse(
      priorResult.response_cache,
      cacheContext,
      receivedAt,
      keyring,
    );
  }
  const wire = challengeState.challenge;
  const scope = {
    type: "AUTH_SECURITY",
    source_key: challengeState.source_key,
    user_id: challengeState.user_id,
  };
  const aggregateId = syntheticUuid(`challenge:${wire.subject_handle}`);
  const correlationId = syntheticUuid(
    `enrollment-correlation:${wire.subject_handle}`,
  );
  if (challengeState.status === "CONSUMED") {
    persistEnrollmentAudit(state, {
      kind: "ENROLLMENT_PROOF_REJECTED",
      scope,
      requestId: challengeState.request_id,
      correlationId,
      occurredAt: receivedAt,
      aggregateId,
    });
    return recordEnrollmentResponse(
      state,
      cacheContext,
      "REJECT_REPLAY",
      receivedAt,
      keyring,
    );
  }
  if (challengeState.status === "EXPIRED") {
    return recordEnrollmentResponse(
      state,
      cacheContext,
      "REJECT_EXPIRED",
      receivedAt,
      keyring,
    );
  }
  if (isoMilliseconds(receivedAt) >= isoMilliseconds(wire.expires_at)) {
    challengeState.status = "EXPIRED";
    challengeState.expired_at = receivedAt;
    assert(validatorFor("EnrollmentChallengeState")(challengeState));
    persistEnrollmentAudit(state, {
      kind: "ENROLLMENT_PROOF_REJECTED",
      scope,
      requestId: challengeState.request_id,
      correlationId,
      occurredAt: receivedAt,
      aggregateId,
    });
    return recordEnrollmentResponse(
      state,
      cacheContext,
      "REJECT_EXPIRED",
      receivedAt,
      keyring,
    );
  }
  challengeState.status = "CONSUMED";
  challengeState.consumed_at = receivedAt;
  assert(validatorFor("EnrollmentChallengeState")(challengeState));
  if (!enrollmentProofIsValid(wire, completion)) {
    persistEnrollmentAudit(state, {
      kind: "ENROLLMENT_PROOF_REJECTED",
      scope,
      requestId: challengeState.request_id,
      correlationId,
      occurredAt: receivedAt,
      aggregateId,
    });
    return recordEnrollmentResponse(
      state,
      cacheContext,
      "CONSUMED_INVALID_PROOF",
      receivedAt,
      keyring,
    );
  }
  if (
    !activeOwnership(
      state,
      challengeState.user_id,
      challengeState.trading_account_id,
    )
  ) {
    persistEnrollmentAudit(state, {
      kind: "ENROLLMENT_PROOF_REJECTED",
      scope,
      requestId: challengeState.request_id,
      correlationId,
      occurredAt: receivedAt,
      aggregateId,
    });
    return recordEnrollmentResponse(
      state,
      cacheContext,
      "REJECT_INACTIVE_OWNERSHIP",
      receivedAt,
      keyring,
    );
  }
  const existingOwner = state.publicKeyOwners.get(
    wire.candidate_public_key_fingerprint,
  );
  if (existingOwner !== undefined) {
    const outcome =
      existingOwner === challengeState.user_id
        ? "REJECT_DUPLICATE_KEY"
        : "REJECT_CROSS_OWNER_REBIND";
    persistEnrollmentAudit(state, {
      kind: "ENROLLMENT_PROOF_REJECTED",
      scope,
      requestId: challengeState.request_id,
      correlationId,
      occurredAt: receivedAt,
      aggregateId,
    });
    return recordEnrollmentResponse(
      state,
      cacheContext,
      outcome,
      receivedAt,
      keyring,
    );
  }
  if (wire.purpose === "REPLACEMENT_DEVICE") {
    for (const record of state.devices.values()) {
      if (
        record.user_id === challengeState.user_id &&
        record.status === "ACTIVE"
      ) {
        record.status = "REVOKED";
        record.revoked_at = receivedAt;
        assert(validatorFor("Device")(record));
      }
    }
    for (const record of state.sessions.values()) {
      if (
        record.user_id === challengeState.user_id &&
        record.status === "ACTIVE"
      ) {
        record.status = "REVOKED";
        record.revoked_at = receivedAt;
        assert(validatorFor("Session")(record));
      }
    }
    for (const record of state.refreshFamilies.values()) {
      if (
        record.user_id === challengeState.user_id &&
        record.state.status === "ACTIVE"
      ) {
        record.state.status = "REVOKED";
        for (const token of record.state.tokens) {
          token.status = "REVOKED";
          token.revoked_at = receivedAt;
        }
        assert(validatorFor("RefreshFamilyState")(record.state));
      }
    }
  }
  const deviceId = syntheticUuid(`device:${wire.subject_handle}`);
  const sessionId = syntheticUuid(`session:${wire.subject_handle}`);
  const familyId = syntheticUuid(`family:${wire.subject_handle}`);
  const enrollmentId = syntheticUuid(`enrollment:${wire.subject_handle}`);
  const familyDeadline = new Date(isoMilliseconds(receivedAt) + 2_592_000_000)
    .toISOString()
    .replace(".000Z", "Z");
  const refreshExpiresAt = new Date(
    Math.min(
      isoMilliseconds(receivedAt) + 604_800_000,
      isoMilliseconds(familyDeadline),
    ),
  )
    .toISOString()
    .replace(".000Z", "Z");
  const device = {
    schema_version: "fit.platform.device.v1",
    device_id: deviceId,
    user_id: challengeState.user_id,
    public_key_fingerprint: wire.candidate_public_key_fingerprint,
    key_algorithm: "Ed25519",
    status: "ACTIVE",
    enrolled_at: receivedAt,
  };
  const session = {
    schema_version: "fit.platform.session.v1",
    session_id: sessionId,
    user_id: challengeState.user_id,
    trading_account_id: challengeState.trading_account_id,
    device_id: deviceId,
    refresh_family_id: familyId,
    status: "ACTIVE",
    created_at: receivedAt,
    access_expires_at: new Date(isoMilliseconds(receivedAt) + 900_000)
      .toISOString()
      .replace(".000Z", "Z"),
  };
  const refreshFamily = {
    schema_version: "fit.platform.refresh-family-state.v1",
    family_id: familyId,
    session_id: sessionId,
    family_created_at: receivedAt,
    family_deadline: familyDeadline,
    status: "ACTIVE",
    tokens: [
      {
        schema_version: "fit.platform.refresh-record.v1",
        token_digest: sha256(`refresh:${wire.subject_handle}`),
        family_id: familyId,
        session_id: sessionId,
        family_created_at: receivedAt,
        issued_at: receivedAt,
        expires_at: refreshExpiresAt,
        family_deadline: familyDeadline,
        status: "ACTIVE",
      },
    ],
  };
  const enrollment = {
    schema_version: "fit.platform.device-enrollment.v1",
    enrollment_id: enrollmentId,
    purpose: wire.purpose,
    user_id: challengeState.user_id,
    device_id: deviceId,
    first_session_id: sessionId,
    subject_handle_digest: sha256(wire.subject_handle),
    candidate_public_key_fingerprint: wire.candidate_public_key_fingerprint,
    proof_algorithm: "Ed25519",
    prior_scope_effect:
      wire.purpose === "REPLACEMENT_DEVICE"
        ? "REVOKE_ALL_BEFORE_CREATE"
        : "PRESERVE_EXISTING",
    completed_at: receivedAt,
    identity_origin: "SERVER_AUTHORED",
  };
  for (const [schemaName, value] of [
    ["Device", device],
    ["Session", session],
    ["RefreshFamilyState", refreshFamily],
    ["DeviceEnrollment", enrollment],
  ]) {
    assert(
      validatorFor(schemaName)(value),
      `generated ${schemaName} invalid: ${ajv.errorsText(
        validatorFor(schemaName).errors,
      )}`,
    );
  }
  state.publicKeyOwners.set(
    wire.candidate_public_key_fingerprint,
    challengeState.user_id,
  );
  state.devices.set(device.device_id, device);
  state.sessions.set(session.session_id, session);
  state.refreshFamilies.set(refreshFamily.family_id, {
    user_id: challengeState.user_id,
    state: refreshFamily,
  });
  state.enrollments.set(enrollment.enrollment_id, enrollment);
  const eventKind =
    wire.purpose === "REPLACEMENT_DEVICE"
      ? "DEVICE_REPLACED"
      : "DEVICE_ENROLLED";
  persistEnrollmentAudit(state, {
    kind: eventKind,
    scope,
    requestId: challengeState.request_id,
    correlationId,
    occurredAt: receivedAt,
    aggregateId: enrollmentId,
    deviceId,
  });
  persistEnrollmentNotification(state, {
    kind: eventKind,
    scope,
    requestId: challengeState.request_id,
    correlationId,
    occurredAt: receivedAt,
    aggregateId: enrollmentId,
    deviceId,
  });
  return recordEnrollmentResponse(
    state,
    cacheContext,
    "CONSUMED_SUCCESS",
    receivedAt,
    keyring,
    {
      device_id: deviceId,
      session_id: sessionId,
      refresh_family_id: familyId,
      access_token_transport: `synthetic-access-token:${sessionId}`,
      refresh_token_transport: `synthetic-refresh-token:${familyId}`,
    },
  );
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
  return item.proof_valid ? "CONSUMED_SUCCESS" : "CONSUMED_INVALID_PROOF";
}

function outcomeOf(result) {
  return result !== null && typeof result === "object"
    ? result.outcome
    : result;
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

function presentRefreshToken(
  runtime,
  tokenDigest,
  now,
  descendantDigest,
  requestKey,
  keyring,
) {
  if (
    runtime === null ||
    typeof runtime !== "object" ||
    typeof runtime.userId !== "string" ||
    typeof runtime.tradingAccountId !== "string" ||
    !(runtime.responseLedger instanceof Map) ||
    !Array.isArray(runtime.securityEffects) ||
    !validatorFor("RefreshFamilyState")(runtime.family) ||
    typeof tokenDigest !== "string" ||
    !/^[a-f0-9]{64}$/u.test(tokenDigest) ||
    typeof requestKey !== "string" ||
    !(keyring instanceof Map)
  ) {
    return "REJECT_MALFORMED";
  }
  try {
    isoMilliseconds(now);
    canonicalUuid(requestKey);
    canonicalUuid(runtime.userId);
    canonicalUuid(runtime.tradingAccountId);
  } catch {
    return "REJECT_MALFORMED";
  }
  const requestDigest = authenticatedMutationDigest({
    routeTemplate: "/v1/auth/refresh",
    body: { token_digest: tokenDigest },
    userId: runtime.userId,
    tradingAccountId: runtime.tradingAccountId,
  });
  const staged = structuredClone(runtime);
  const result = presentRefreshTokenTransaction(
    staged,
    tokenDigest,
    now,
    descendantDigest,
    requestKey,
    requestDigest,
    keyring,
  );
  replaceRuntimeState(runtime, staged);
  return result;
}

function presentRefreshTokenTransaction(
  runtime,
  tokenDigest,
  now,
  descendantDigest,
  requestKey,
  requestDigest,
  keyring,
) {
  const family = runtime.family;
  const cacheContext = responseCacheContext({
    userId: runtime.userId,
    tradingAccountId: runtime.tradingAccountId,
    routeTemplate: "/v1/auth/refresh",
    requestKey,
    requestDigest,
  });
  const prior = runtime.responseLedger.get(responseLedgerKey(cacheContext));
  if (prior !== undefined) {
    if (prior.requestDigest !== requestDigest) {
      return "IDEMPOTENCY_CONFLICT";
    }
    if (
      prior.response_cache === undefined ||
      cacheTimeMilliseconds(now) >= prior.response_cache.expires_at_ms
    ) {
      expireCachedResponse(prior, now);
      return "RECONCILIATION_REQUIRED_CACHE_EXPIRED";
    }
    return openCachedResponse(
      prior.response_cache,
      cacheContext,
      now,
      keyring,
    );
  }
  const token = family.tokens.find(
    ({ token_digest: digest }) => digest === tokenDigest,
  );
  if (token === undefined) {
    return "REJECT_UNKNOWN_TOKEN_GENERIC";
  }
  const outcome = refreshOutcome({
    status: token.status,
    now,
    individual_expires_at: token.expires_at,
    family_deadline: family.family_deadline,
  });
  if (outcome === "ROTATE_CREATE_DESCENDANT") {
    if (
      typeof descendantDigest !== "string" ||
      !/^[a-f0-9]{64}$/u.test(descendantDigest) ||
      descendantDigest === tokenDigest ||
      family.tokens.some(({ token_digest: digest }) => digest === descendantDigest)
    ) {
      return "REJECT_SERVER_RANDOMNESS_UNAVAILABLE";
    }
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
    assert(validatorFor("RefreshFamilyState")(family));
    const response = {
      replay_classification: "RETURN_RECORDED_ROTATION",
      outcome,
      descendant_digest: descendantDigest,
      refresh_token_transport: `synthetic-refresh-token:${descendantDigest}`,
    };
    runtime.responseLedger.set(responseLedgerKey(cacheContext), {
      requestDigest,
      response_cache: sealCachedResponse(
        response,
        cacheContext,
        now,
        keyring,
      ),
    });
    return response;
  } else if (outcome === "REVOKE_FAMILY_AND_DESCENDANTS") {
    family.status = "REVOKED";
    for (const member of family.tokens) {
      member.status = "REVOKED";
      member.revoked_at = now;
    }
    assert(validatorFor("RefreshFamilyState")(family));
    runtime.securityEffects.push(
      {
        type: "AUDIT",
        kind: "REFRESH_REUSE_DETECTED",
        family_id: family.family_id,
      },
      {
        type: "NOTIFICATION",
        kind: "REFRESH_REUSE_DETECTED",
        family_id: family.family_id,
      },
      {
        type: "OUTBOX",
        kind: "REFRESH_REUSE_DETECTED",
        family_id: family.family_id,
      },
    );
    const response = {
      replay_classification: "RETURN_RECORDED_REUSE_REVOCATION",
      outcome,
      family_id: family.family_id,
    };
    runtime.responseLedger.set(responseLedgerKey(cacheContext), {
      requestDigest,
      response_cache: sealCachedResponse(
        response,
        cacheContext,
        now,
        keyring,
      ),
    });
    return response;
  }
  return outcome;
}

function refreshRuntimeFor(family) {
  return {
    userId: "10000000-0000-4000-8000-000000000001",
    tradingAccountId: "20000000-0000-4000-8000-000000000001",
    family: clone(family),
    responseLedger: new Map(),
    securityEffects: [],
  };
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
    item.user_active === false ||
    item.account_ownership_active === false ||
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
  try {
    if (item.name === "duplicate query key") {
      parseStrictRequestTarget(
        "/v1/confirmations/70000000-0000-4000-8000-000000000001/consume?mode=a&mode=b",
      );
    } else if (item.name === "non JSON mutation") {
      parseStrictJson("not-json");
    } else if (item.name === "unknown body field") {
      assert(
        validatorFor("ConfirmationConsumeMutationInput")({
          idempotency_key: "60000000-0000-4000-8000-000000000001",
          path: {
            confirmation_id:
              "70000000-0000-4000-8000-000000000001",
          },
          query: {},
          body: {
            confirmation_hash: "a".repeat(64),
            undeclared_switch: true,
          },
        }),
      );
    } else if (item.name === "ambiguous normalized path") {
      parseStrictRequestTarget(
        "/v1/confirmations/../70000000-0000-4000-8000-000000000001/consume",
      );
    } else if (item.name === "strictly decoded mutation") {
      parseStrictRequestTarget(
        "/v1/confirmations/70000000-0000-4000-8000-000000000001/consume",
      );
      const decoded = parseStrictJson(
        `{"idempotency_key":"60000000-0000-4000-8000-000000000001","path":{"confirmation_id":"70000000-0000-4000-8000-000000000001"},"query":{},"body":{"confirmation_hash":"${"a".repeat(64)}"}}`,
      );
      assert(validatorFor("ConfirmationConsumeMutationInput")(decoded));
    } else {
      assert.fail(`unknown strict request scenario ${item.name}`);
    }
    return "ACCEPT";
  } catch {
    return "REJECT";
  }
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

function cloneThrottleDimension(state) {
  return {
    failureTimesMs: [...state.failureTimesMs],
    failures: state.failures,
    nextAllowedMs: state.nextAllowedMs,
    lockedUntilMs: state.lockedUntilMs,
  };
}

function runThrottleTimeline(timeline, throttle) {
  const sources = new Map();
  const accounts = new Map();
  for (const event of timeline.events) {
    const nowMs = event.at_seconds * 1000;
    const sourceKey = event.source_key ?? timeline.source_key;
    const storedSource = sources.get(sourceKey) ?? newThrottleDimension();
    const source = cloneThrottleDimension(storedSource);
    const storedAccount =
      event.user_id === null
        ? null
        : (accounts.get(event.user_id) ?? newThrottleDimension());
    const account =
      storedAccount === null ? null : cloneThrottleDimension(storedAccount);
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
          const dimensionDelaySeconds =
            throttle.failure_delays_seconds[state.failures - 1];
          appliedDelaySeconds = Math.max(
            appliedDelaySeconds,
            dimensionDelaySeconds,
          );
          state.nextAllowedMs = nowMs + dimensionDelaySeconds * 1000;
        }
      }
      decision = dimensions.some((state) => state.lockedUntilMs > nowMs)
        ? "LOCKED"
        : "FAILED_DELAY";
    }
    const observedSource =
      event.transaction_aborted === true ? storedSource : source;
    const observedAccount =
      event.transaction_aborted === true ? storedAccount : account;
    if (event.transaction_aborted !== true) {
      sources.set(sourceKey, source);
      if (account !== null) accounts.set(event.user_id, account);
    }
    assert.equal(
      decision,
      event.expected_decision,
      `${timeline.name} decision`,
    );
    assert.equal(
      observedAccount?.failures ?? 0,
      event.expected_account_failures,
      `${timeline.name} account failures`,
    );
    assert.equal(
      observedSource.failures,
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
        observedSource.lockedUntilMs / 1000,
        event.expected_lock_until_seconds,
        `${timeline.name} source lock expiry`,
      );
      if (observedAccount !== null) {
        assert.equal(
          observedAccount.lockedUntilMs / 1000,
          event.expected_lock_until_seconds,
          `${timeline.name} account lock expiry`,
        );
      }
    }
    if (Object.hasOwn(event, "expected_source_lock_until_seconds")) {
      assert.equal(
        observedSource.lockedUntilMs / 1000,
        event.expected_source_lock_until_seconds,
        `${timeline.name} source-only lock expiry`,
      );
    }
    if (Object.hasOwn(event, "expected_account_lock_until_seconds")) {
      assert.equal(
        observedAccount?.lockedUntilMs / 1000,
        event.expected_account_lock_until_seconds,
        `${timeline.name} account-only lock expiry`,
      );
    }
    if (Object.hasOwn(event, "expected_source_next_allowed_seconds")) {
      assert.equal(
        observedSource.nextAllowedMs / 1000,
        event.expected_source_next_allowed_seconds,
        `${timeline.name} source next allowed`,
      );
    }
    if (Object.hasOwn(event, "expected_account_next_allowed_seconds")) {
      assert.equal(
        observedAccount?.nextAllowedMs / 1000,
        event.expected_account_next_allowed_seconds,
        `${timeline.name} account next allowed`,
      );
    }
    if (Object.hasOwn(event, "expected_source_failure_times_seconds")) {
      assertDeepEqual(
        observedSource.failureTimesMs.map((value) => value / 1000),
        event.expected_source_failure_times_seconds,
        `${timeline.name} source failure timestamps`,
      );
    }
  }
  return { sources, accounts };
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
  if (!value.includes(":")) {
    const result = Buffer.alloc(16);
    result[10] = 0xff;
    result[11] = 0xff;
    parseIpv4(value).copy(result, 12);
    return result;
  }

  let ipv6Text = value;
  if (value.includes(".")) {
    const finalColon = value.lastIndexOf(":");
    assert(finalColon >= 0, `invalid mixed IPv6 address ${value}`);
    const octets = parseIpv4(value.slice(finalColon + 1));
    const high = ((octets[0] << 8) | octets[1]).toString(16);
    const low = ((octets[2] << 8) | octets[3]).toString(16);
    ipv6Text = `${value.slice(0, finalColon + 1)}${high}:${low}`;
  }

  assert(
    /^[0-9a-fA-F:]+$/.test(ipv6Text) && !ipv6Text.includes(":::"),
    `invalid IPv6 address ${value}`,
  );
  const doubleColonParts = ipv6Text.split("::");
  assert(doubleColonParts.length <= 2, `invalid IPv6 address ${value}`);
  const left = doubleColonParts[0] === "" ? [] : doubleColonParts[0].split(":");
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
  if (item.incident_open_before && item.consecutive_healthy_cycles + 1 >= 2) {
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
  websocketProtocol,
  phase0Manifest,
  phase0DomainSchema,
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
    path.join(platformDirectory, "schemas", "recovery-evidence-v1.schema.json"),
  ),
  readJson(path.join(fixtureDirectory, "valid-schema-cases-v1.json")),
  readJson(path.join(fixtureDirectory, "invalid-schema-cases-v1.json")),
  readJson(path.join(fixtureDirectory, "semantic-scenarios-v1.json")),
  readJson(
    path.join(fixtureDirectory, "executable-security-scenarios-v1.json"),
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
    path.join(platformDirectory, "manifests", "transaction-boundaries-v1.json"),
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
  readJson(path.join(platformDirectory, "manifests", "state-machines-v1.json")),
  readJson(
    path.join(platformDirectory, "manifests", "websocket-protocol-v1.json"),
  ),
  readJson(
    path.join(platformDirectory, "manifests", "phase0-file-manifest-v1.json"),
  ),
  readJson(
    path.join(contractsDirectory, "jsonschema", "fit-trade-v1.schema.json"),
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
  keyword: "x-fit-request-digest",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    try {
      return data.canonical_request_digest === canonicalRequestDigest(data);
    } catch {
      return false;
    }
  },
});
ajv.addKeyword({
  keyword: "x-fit-enrollment-challenge-state",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    try {
      return (
        data.subject_handle_digest === sha256(data.challenge.subject_handle) &&
        data.created_at === data.challenge.issued_at &&
        (data.status === "ACTIVE"
          ? !Object.hasOwn(data, "consumed_at") &&
            !Object.hasOwn(data, "expired_at")
          : data.status === "CONSUMED" &&
              Date.parse(data.consumed_at) >= Date.parse(data.created_at) &&
              !Object.hasOwn(data, "expired_at") ||
            data.status === "EXPIRED" &&
              Date.parse(data.expired_at) >=
                Date.parse(data.challenge.expires_at) &&
              !Object.hasOwn(data, "consumed_at"))
      );
    } catch {
      return false;
    }
  },
});
ajv.addKeyword({
  keyword: "x-fit-event-payload",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    try {
      const durable = data.data;
      const record = durable.record;
      const subjectBinding = nats.subject_bindings.find(
        ({ subject }) => subject === data.subject,
      );
      return (
        subjectBinding !== undefined &&
        subjectBinding.event_kind === data.event_kind &&
        subjectBinding.scope === data.scope.type &&
        data.event_identity.event_id === durable.record_id &&
        data.event_identity.previous_event_id !== data.event_identity.event_id &&
        record.kind === data.event_kind &&
        jcsCanonicalize(record.scope) === jcsCanonicalize(data.scope) &&
        (durable.record_type === "audit"
          ? !data.subject.includes(".notification.") &&
            record.event_id === durable.record_id
          : durable.record_type === "notification" &&
            data.subject.includes(".notification.") &&
            record.notification_id === durable.record_id)
      );
    } catch {
      return false;
    }
  },
});
ajv.addKeyword({
  keyword: "x-fit-durable-record-digest",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    try {
      return data.payload_digest === durableRecordDigest(data);
    } catch {
      return false;
    }
  },
});
ajv.addKeyword({
  keyword: "x-fit-aggregate-history",
  schemaType: "boolean",
  type: "object",
  errors: false,
  validate: (_rule, data) => {
    try {
      const eventIds = new Set();
      const versions = new Set();
      let prior = null;
      for (const [index, event] of data.events.entries()) {
        if (
          event.aggregate_type !== data.aggregate_type ||
          event.aggregate_id !== data.aggregate_id ||
          event.aggregate_version !== data.starting_version + index ||
          eventIds.has(event.event_id) ||
          versions.has(event.aggregate_version)
        ) {
          return false;
        }
        eventIds.add(event.event_id);
        versions.add(event.aggregate_version);
        if (prior === null) {
          if (Object.hasOwn(event, "previous_event_id")) return false;
        } else if (
          event.previous_event_id !== prior.event_id ||
          event.correlation_id !== prior.correlation_id ||
          Date.parse(event.occurred_at) < Date.parse(prior.occurred_at)
        ) {
          return false;
        }
        prior = event;
      }
      return true;
    } catch {
      return false;
    }
  },
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
          Number.isFinite(startMs) && Number.isFinite(endMs) && endMs >= startMs
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
      const byDigest = new Map(
        tokens.map((token) => [token.token_digest, token]),
      );
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
            Date.parse(descendant.issued_at) < Date.parse(token.issued_at) ||
            Date.parse(descendant.issued_at) >= Date.parse(token.expires_at) ||
            Date.parse(descendant.issued_at) >= Date.parse(data.family_deadline)
          ) {
            return false;
          }
          if (referencedDigests.has(token.rotated_to_digest)) return false;
          referencedDigests.add(token.rotated_to_digest);
        }
        if (data.status === "REVOKED" && token.status !== "REVOKED")
          return false;
      }
      const roots = tokens.filter(
        ({ token_digest: digest }) => !referencedDigests.has(digest),
      );
      if (roots.length !== 1 || referencedDigests.size !== tokens.length - 1) {
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
        data.payload.event_identity.event_id === data.event_id &&
        data.payload.event_identity.aggregate_type === data.aggregate_type &&
        data.payload.event_identity.aggregate_id === data.aggregate_id &&
        data.payload.event_identity.aggregate_version ===
          data.aggregate_version &&
        (Object.hasOwn(data, "previous_event_id")
          ? data.payload.event_identity.previous_event_id ===
            data.previous_event_id
          : !Object.hasOwn(
              data.payload.event_identity,
              "previous_event_id",
            )) &&
        sha256(jcsCanonicalize(data.payload)) === data.payload_digest &&
        data.payload.data.record.kind === data.event_kind &&
        jcsCanonicalize(data.payload.data.record.scope) ===
          jcsCanonicalize(data.scope) &&
        data.payload.data.record.causation_id === data.causation_id &&
        data.payload.data.record.correlation_id === data.correlation_id &&
        data.payload.data.record.occurred_at === data.occurred_at &&
        data.payload.data.record.payload_digest ===
          durableRecordDigest(data.payload.data.record) &&
        (data.payload.data.record_type === "audit"
          ? !data.subject.includes(".notification.") &&
            data.payload.data.record.event_id === data.payload.data.record_id
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
        [committedAt, recoveredAt, injectedAt, invokedAt, terminalAt].every(
          Number.isFinite,
        ) &&
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
const retentionNegativeFixture = invalidFixtureSet.cases.find(
  ({ name }) => name === "outbox rejects deletion retention",
);
const retentionOnlyRepair = clone(retentionNegativeFixture.value);
retentionOnlyRepair.retention = "NEVER_DELETE_IN_PHASE_1";
assert(
  validatorFor("OutboxRecord")(retentionOnlyRepair),
  "Outbox retention negative fixture is not isolated to retention",
);

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
  security.server_authored_identity.authenticated_http_input_schemas.default,
  "UntrustedMutationInput",
);
assert.equal(
  security.server_authored_identity.authenticated_http_input_schemas[
    "POST /v1/confirmations/{confirmation_id}/consume"
  ],
  "ConfirmationConsumeMutationInput",
);
assert.equal(
  security.server_authored_identity.model_tool_input_schema,
  "ModelToolMutationInput",
);
assert.equal(
  security.challenge.enrollment.start_input_schema,
  "EnrollmentStartInput",
);
assertDeepEqual(security.challenge.enrollment.client_start_fields, [
  "identifier",
  "password_utf8",
  "purpose",
  "candidate_public_key_fingerprint",
]);
assertDeepEqual(security.challenge.enrollment.server_issuance_fields, [
  "subject_handle",
  "nonce",
  "issued_at",
  "expires_at",
]);
assert.equal(
  security.challenge.enrollment.ownership_check,
  "ACTIVE_OWNER_REQUIRED_AT_START_AND_COMPLETION",
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
const validateConfirmationConsumeMutation = validatorFor(
  "ConfirmationConsumeMutationInput",
);
assert(
  validateConfirmationConsumeMutation(legitimateConfirmationInput),
  "HTTP confirmation input does not match its exact route schema",
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
const validRequestEnvelope = validFixtureSet.cases.find(
  ({ schema }) => schema === "RequestEnvelope",
).value;
assert.equal(
  validRequestEnvelope.route_template,
  "/v1/confirmations/{confirmation_id}/consume",
);
assert.equal(
  validRequestEnvelope.canonical_request_digest,
  canonicalRequestDigest(validRequestEnvelope),
);
for (const mutate of [
  (value) => {
    value.method = "PUT";
  },
  (value) => {
    value.route_template = "/v1/confirmations/{confirmation_id}/other";
  },
  (value) => {
    value.path.confirmation_id = "70000000-0000-4000-8000-000000000002";
  },
  (value) => {
    value.query.mode = "strict";
  },
  (value) => {
    value.body.confirmation_hash = "b".repeat(64);
  },
  (value) => {
    value.user_id = "10000000-0000-4000-8000-000000000002";
  },
  (value) => {
    value.trading_account_id = "20000000-0000-4000-8000-000000000002";
  },
]) {
  const staleDigestEnvelope = clone(validRequestEnvelope);
  mutate(staleDigestEnvelope);
  assert(
    !validatorFor("RequestEnvelope")(staleDigestEnvelope),
    "RequestEnvelope accepted a stale canonical digest",
  );
}
for (const mutate of [
  (value) => {
    value.query.mode = "strict";
  },
  (value) => {
    value.body.undeclared_switch = true;
  },
  (value) => {
    value.path.undeclared_identifier = "synthetic";
  },
]) {
  const adversarialEnvelope = clone(validRequestEnvelope);
  mutate(adversarialEnvelope);
  adversarialEnvelope.canonical_request_digest =
    canonicalRequestDigest(adversarialEnvelope);
  assert(
    !validatorFor("RequestEnvelope")(adversarialEnvelope),
    "RequestEnvelope accepted a digest-consistent undeclared route input",
  );
}
for (const mutate of [
  (value) => {
    value.query.mode = "strict";
  },
  (value) => {
    value.body.undeclared_switch = true;
  },
  (value) => {
    value.path.undeclared_identifier = "synthetic";
  },
]) {
  const adversarialInput = clone(legitimateConfirmationInput);
  mutate(adversarialInput);
  assert(
    !validateConfirmationConsumeMutation(adversarialInput),
    "confirmation route accepted an undeclared path/query/body field",
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
      fixture.schema === "EnrollmentChallenge" ? "enrollment" : "device_action"
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
    value.tokens[1].family_id = "50000000-0000-4000-8000-000000000099";
  },
  (value) => {
    value.tokens[1].status = "ROTATED";
    value.tokens[1].rotated_to_digest = value.tokens[0].token_digest;
  },
  (value) => {
    value.status = "REVOKED";
  },
  (value) => {
    value.tokens[1].issued_at = value.tokens[0].expires_at;
    value.tokens[1].expires_at = "2026-08-12T10:00:00Z";
  },
]) {
  const invalidFamily = clone(validRefreshFamily);
  mutate(invalidFamily);
  assert(
    !validateRefreshFamily(invalidFamily),
    "refresh family accepted cross-family, cyclic, post-expiry, or partially revoked lineage",
  );
}
const executableRefreshRuntime = refreshRuntimeFor({
  schema_version: "fit.platform.refresh-family-state.v1",
  family_id: validRefreshRecord.family_id,
  session_id: validRefreshRecord.session_id,
  family_created_at: validRefreshRecord.family_created_at,
  family_deadline: validRefreshRecord.family_deadline,
  status: "ACTIVE",
  tokens: [clone(validRefreshRecord)],
});
assert.equal(
  outcomeOf(presentRefreshToken(
    executableRefreshRuntime,
    validRefreshRecord.token_digest,
    "2026-08-01T10:00:00Z",
    "b".repeat(64),
    "60000000-0000-4000-8000-000000000019",
    primaryResponseCacheKeyring,
  )),
  "ROTATE_CREATE_DESCENDANT",
);
assert(validateRefreshFamily(executableRefreshRuntime.family));
assert.equal(
  outcomeOf(presentRefreshToken(
    executableRefreshRuntime,
    validRefreshRecord.token_digest,
    "2026-08-02T10:00:00Z",
    undefined,
    "60000000-0000-4000-8000-000000000020",
    primaryResponseCacheKeyring,
  )),
  "REVOKE_FAMILY_AND_DESCENDANTS",
);
assert(validateRefreshFamily(executableRefreshRuntime.family));
assert(
  executableRefreshRuntime.family.tokens.every(
    ({ status }) => status === "REVOKED",
  ),
  "refresh reuse did not revoke every already-issued descendant",
);
assert.equal(executableRefreshRuntime.securityEffects.length, 3);
const responseLossRefreshRuntime = {
  userId: "10000000-0000-4000-8000-000000000001",
  tradingAccountId: "20000000-0000-4000-8000-000000000001",
  family: {
    schema_version: "fit.platform.refresh-family-state.v1",
    family_id: validRefreshRecord.family_id,
    session_id: validRefreshRecord.session_id,
    family_created_at: validRefreshRecord.family_created_at,
    family_deadline: validRefreshRecord.family_deadline,
    status: "ACTIVE",
    tokens: [clone(validRefreshRecord)],
  },
  responseLedger: new Map(),
  securityEffects: [],
};
const refreshRequestKey = "60000000-0000-4000-8000-000000000021";
const refreshRequestDigest = authenticatedMutationDigest({
  routeTemplate: "/v1/auth/refresh",
  body: { token_digest: validRefreshRecord.token_digest },
  userId: responseLossRefreshRuntime.userId,
  tradingAccountId: responseLossRefreshRuntime.tradingAccountId,
});
const refreshCacheContext = responseCacheContext({
  userId: responseLossRefreshRuntime.userId,
  tradingAccountId: responseLossRefreshRuntime.tradingAccountId,
  routeTemplate: "/v1/auth/refresh",
  requestKey: refreshRequestKey,
  requestDigest: refreshRequestDigest,
});
const firstRefreshResponse = presentRefreshToken(
  responseLossRefreshRuntime,
  validRefreshRecord.token_digest,
  "2026-08-01T10:00:00Z",
  "c".repeat(64),
  refreshRequestKey,
  primaryResponseCacheKeyring,
);
assert.equal(outcomeOf(firstRefreshResponse), "ROTATE_CREATE_DESCENDANT");
const refreshCacheEntry =
  responseLossRefreshRuntime.responseLedger.get(
    responseLedgerKey(refreshCacheContext),
  );
assert(
  !JSON.stringify(refreshCacheEntry).includes("synthetic-refresh-token:"),
  "refresh response cache persisted plaintext credential material",
);
assertDeepEqual(
  openCachedResponse(
    refreshCacheEntry.response_cache,
    refreshCacheContext,
    "2026-08-01T10:00:01Z",
    primaryResponseCacheKeyring,
  ),
  {
    descendant_digest: "c".repeat(64),
    outcome: "ROTATE_CREATE_DESCENDANT",
    refresh_token_transport: `synthetic-refresh-token:${"c".repeat(64)}`,
    replay_classification: "RETURN_RECORDED_ROTATION",
  },
  "refresh response cache cannot recover the exact committed rotation",
);
assertDeepEqual(
  openCachedResponse(
    refreshCacheEntry.response_cache,
    refreshCacheContext,
    "2026-08-01T10:00:01Z",
    replicaResponseCacheKeyring,
  ),
  firstRefreshResponse,
  "a replica with the same deployment keyring could not recover the cache",
);
const crossScopeContext = responseCacheContext({
  userId: "10000000-0000-4000-8000-000000000099",
  tradingAccountId: "20000000-0000-4000-8000-000000000099",
  routeTemplate: "/v1/auth/refresh",
  requestKey: refreshRequestKey,
  requestDigest: refreshRequestDigest,
});
assert.throws(
  () =>
    openCachedResponse(
      refreshCacheEntry.response_cache,
      crossScopeContext,
      "2026-08-01T10:00:01Z",
      primaryResponseCacheKeyring,
    ),
  /scope or digest binding mismatch/u,
);
const transplantedCache = clone(refreshCacheEntry.response_cache);
transplantedCache.user_id = crossScopeContext.user_id;
transplantedCache.trading_account_id =
  crossScopeContext.trading_account_id;
assert.throws(
  () =>
    openCachedResponse(
      transplantedCache,
      crossScopeContext,
      "2026-08-01T10:00:01Z",
      primaryResponseCacheKeyring,
    ),
  /auth|Unsupported state/u,
  "cross-scope ciphertext transplant passed GCM authentication",
);
const extendedTtlCache = clone(refreshCacheEntry.response_cache);
extendedTtlCache.expires_at_ms += 60_000;
assert.throws(
  () =>
    openCachedResponse(
      extendedTtlCache,
      refreshCacheContext,
      "2026-08-01T10:02:01Z",
      primaryResponseCacheKeyring,
    ),
  /auth|Unsupported state/u,
  "unauthenticated TTL extension opened expired ciphertext",
);
const changedDigestContext = {
  ...refreshCacheContext,
  request_digest: "f".repeat(64),
};
assert.throws(
  () =>
    openCachedResponse(
      refreshCacheEntry.response_cache,
      changedDigestContext,
      "2026-08-01T10:00:01Z",
      primaryResponseCacheKeyring,
    ),
  /scope or digest binding mismatch/u,
);
assertDeepEqual(
  presentRefreshToken(
    responseLossRefreshRuntime,
    validRefreshRecord.token_digest,
    "2026-08-01T10:00:00Z",
    "c".repeat(64),
    refreshRequestKey,
    primaryResponseCacheKeyring,
  ),
  firstRefreshResponse,
  "same-scope refresh retry did not return the exact credential response",
);
assert.equal(responseLossRefreshRuntime.family.status, "ACTIVE");
assert.equal(responseLossRefreshRuntime.family.tokens.length, 2);
assertDeepEqual(
  presentRefreshToken(
    responseLossRefreshRuntime,
    validRefreshRecord.token_digest,
    "2026-08-01T10:02:00Z",
    "c".repeat(64),
    refreshRequestKey,
    primaryResponseCacheKeyring,
  ),
  "RECONCILIATION_REQUIRED_CACHE_EXPIRED",
);
assert.equal(responseLossRefreshRuntime.family.tokens.length, 2);
assert.equal(
  responseLossRefreshRuntime.responseLedger.get(
    responseLedgerKey(refreshCacheContext),
  ).response_cache,
  undefined,
  "expired refresh ciphertext was retained",
);
assertDeepEqual(
  presentRefreshToken(
    responseLossRefreshRuntime,
    validRefreshRecord.token_digest,
    "2026-08-02T10:00:00Z",
    undefined,
    "60000000-0000-4000-8000-000000000022",
    primaryResponseCacheKeyring,
  ),
  {
    replay_classification: "RETURN_RECORDED_REUSE_REVOCATION",
    outcome: "REVOKE_FAMILY_AND_DESCENDANTS",
    family_id: validRefreshRecord.family_id,
  },
);
assert.equal(responseLossRefreshRuntime.family.status, "REVOKED");
assert.equal(responseLossRefreshRuntime.securityEffects.length, 3);
assert.equal(
  presentRefreshToken(
    refreshRuntimeFor(
      validFixtureSet.cases.find(
        ({ schema }) => schema === "RefreshFamilyState",
      ).value,
    ),
    "f".repeat(64),
    "2026-08-01T10:00:00Z",
    undefined,
    "60000000-0000-4000-8000-000000000023",
    primaryResponseCacheKeyring,
  ),
  "REJECT_UNKNOWN_TOKEN_GENERIC",
);
assert.equal(
  presentRefreshToken(
    null,
    "f".repeat(64),
    "2026-08-01T10:00:00Z",
    undefined,
    "60000000-0000-4000-8000-000000000024",
    primaryResponseCacheKeyring,
  ),
  "REJECT_MALFORMED",
);
assert.equal(
  presentRefreshToken(
    refreshRuntimeFor(
      validFixtureSet.cases.find(
        ({ schema }) => schema === "RefreshFamilyState",
      ).value,
    ),
    "not-a-digest",
    "not-a-time",
    undefined,
    "60000000-0000-4000-8000-000000000025",
    primaryResponseCacheKeyring,
  ),
  "REJECT_MALFORMED",
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
const validEnrollmentChallengeState = validFixtureSet.cases.find(
  ({ schema }) => schema === "EnrollmentChallengeState",
).value;
for (const mutation of [
  {
    ...clone(validEnrollmentChallengeState),
    subject_handle_digest: "f".repeat(64),
  },
  {
    ...clone(validEnrollmentChallengeState),
    created_at: "2026-07-29T10:00:01Z",
  },
  {
    ...clone(validEnrollmentChallengeState),
    consumed_at: "2026-07-29T10:00:01Z",
  },
  {
    ...clone(validEnrollmentChallengeState),
    status: "CONSUMED",
    consumed_at: "2026-07-29T09:59:59Z",
  },
]) {
  assert(
    !validatorFor("EnrollmentChallengeState")(mutation),
    "EnrollmentChallengeState accepted unbound or contradictory server state",
  );
}
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
    const mutation = {
      ...value,
      [forbiddenField]: "synthetic-forbidden-value",
    };
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
  candidate_public_key: golden.ed25519_proofs.public_key,
  signature: golden.ed25519_proofs.vectors.find(
    ({ challenge_schema }) => challenge_schema === "EnrollmentChallenge",
  ).signature,
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
      candidate_public_key: `ed25519-public:${rawPublicKey.toString("hex")}`,
      signature: `ed25519-signature:${signMessage(
        null,
        signedBytes,
        privateKey,
      ).toString("hex")}`,
    },
  };
}
function startEnrollmentFromFixture(
  state,
  input,
  expectedWire,
  requestKey = syntheticUuid(
    `enrollment-start:${expectedWire.subject_handle}`,
  ),
) {
  const result = startEnrollment(
    state,
    input,
    enrollmentServerContextFromWire(expectedWire),
    requestKey,
    primaryResponseCacheKeyring,
  );
  if (result.wire !== null) {
    assertDeepEqual(
      result.wire,
      expectedWire,
      "server-generated Challenge drifted from the deterministic fixture",
    );
  }
  return result;
}
for (const scenario of executableSecurity.enrollment_transition_cases) {
  assert(
    validatorFor("EnrollmentChallenge")(scenario.wire),
    `synthetic challenge wire is invalid: ${scenario.name}`,
  );
  assert.equal(typeof scenario.input.password_utf8, "string");
  assert(scenario.input.password_utf8.length > 0);
  assert.equal(scenario.wire.purpose, scenario.input.purpose, scenario.name);
  assert.equal(
    scenario.wire.candidate_public_key_fingerprint,
    scenario.input.candidate_public_key_fingerprint,
    `${scenario.name} candidate key was not supplied by the start input`,
  );
  const state = enrollmentStateFromFixture(executableSecurity);
  const startResult = startEnrollmentFromFixture(
    state,
    scenario.input,
    scenario.wire,
  );
  assert.equal(
    startResult.persisted,
    scenario.expected_start_persisted,
    scenario.name,
  );
  const completion = enrollmentCompletionFor(
    startResult.wire,
    scenario.completion.proof_valid
      ? {}
      : { signature: `ed25519-signature:${"0".repeat(128)}` },
  );
  assert(
    validatorFor("EnrollmentCompletionInput")(completion),
    `${scenario.name} completion is not wire-schema valid`,
  );
  assert.equal(
    outcomeOf(
      completeEnrollment(
        state,
        completion,
        "2026-07-29T10:01:00Z",
        syntheticUuid(`enrollment-complete:${scenario.wire.subject_handle}`),
        primaryResponseCacheKeyring,
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
      enrollments: state.enrollments.size,
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
  assert.equal(
    startEnrollment(
      state,
      null,
      enrollmentServerContextFromWire(realEnrollmentScenario.wire),
      syntheticUuid("enrollment-start:null"),
      primaryResponseCacheKeyring,
    ).outcome,
    "REJECT_MALFORMED",
  );
  assert.equal(state.challenges.size, 0);
  assert.equal(
    outcomeOf(completeEnrollment(
      state,
      undefined,
      "2026-07-29T10:01:00Z",
      syntheticUuid("enrollment-complete:undefined"),
      primaryResponseCacheKeyring,
    )),
    "REJECT_MALFORMED",
  );
}
{
  const state = enrollmentStateFromFixture(executableSecurity);
  const ownershipKey =
    "10000000-0000-4000-8000-000000000001:20000000-0000-4000-8000-000000000001";
  state.ownerships.get(ownershipKey).status = "REVOKED";
  state.ownerships.get(ownershipKey).revoked_at = "2026-07-29T09:30:00Z";
  assert.equal(
    startEnrollmentFromFixture(
      state,
      realEnrollmentScenario.input,
      realEnrollmentScenario.wire,
    ).outcome,
    "REJECT_INACTIVE_OWNERSHIP",
  );
  assert.equal(state.challenges.size, 0);
  assert.equal(state.sessions.size, 0);
}
{
  const state = enrollmentStateFromFixture(executableSecurity);
  const startKey = "60000000-0000-4000-8000-000000000041";
  const first = startEnrollmentFromFixture(
    state,
    realEnrollmentScenario.input,
    realEnrollmentScenario.wire,
    startKey,
  );
  const ownership = state.ownerships.get(
    "10000000-0000-4000-8000-000000000001:20000000-0000-4000-8000-000000000001",
  );
  ownership.status = "REVOKED";
  ownership.revoked_at = "2026-07-29T10:00:00Z";
  const alternateIssuance = {
    received_at: "2026-07-29T10:00:01Z",
    subject_handle: "synthetic_alternate_handle_not_committed",
    nonce: "synthetic_alternate_nonce_not_committed",
  };
  const replay = startEnrollment(
    state,
    realEnrollmentScenario.input,
    alternateIssuance,
    startKey,
    primaryResponseCacheKeyring,
  );
  assert.equal(replay.outcome, "CHALLENGE_CREATED");
  assert.equal(
    replay.replay_classification,
    "RETURN_RECORDED_CHALLENGE",
  );
  assertDeepEqual(replay, first);
  assert.equal(state.challenges.size, 1);
  assert.equal(
    startEnrollment(
      state,
      {
        ...realEnrollmentScenario.input,
        candidate_public_key_fingerprint: `ed25519:${"f".repeat(64)}`,
      },
      alternateIssuance,
      startKey,
      primaryResponseCacheKeyring,
    ).outcome,
    "IDEMPOTENCY_CONFLICT",
  );
  const startRouteTemplate = enrollmentRouteTemplate(
    realEnrollmentScenario.input.purpose,
    "start",
  );
  const startDigest = authenticatedMutationDigest({
    routeTemplate: startRouteTemplate,
    body: realEnrollmentScenario.input,
    userId: "10000000-0000-4000-8000-000000000001",
    tradingAccountId: "20000000-0000-4000-8000-000000000001",
  });
  const startCacheContext = responseCacheContext({
    userId: "10000000-0000-4000-8000-000000000001",
    tradingAccountId: "20000000-0000-4000-8000-000000000001",
    routeTemplate: startRouteTemplate,
    requestKey: startKey,
    requestDigest: startDigest,
  });
  assert.equal(
    startEnrollment(
      state,
      realEnrollmentScenario.input,
      {
        ...alternateIssuance,
        received_at: "2026-07-29T10:02:00Z",
      },
      startKey,
      primaryResponseCacheKeyring,
    ).outcome,
    "RECONCILIATION_REQUIRED_CACHE_EXPIRED",
  );
  assert.equal(
    state.responseLedger.get(responseLedgerKey(startCacheContext))
      .response_cache,
    undefined,
    "expired enrollment-start ciphertext was retained",
  );
}
{
  const state = enrollmentStateFromFixture(executableSecurity);
  const sharedKey = "60000000-0000-4000-8000-000000000042";
  const ordinary = startEnrollmentFromFixture(
    state,
    realEnrollmentScenario.input,
    realEnrollmentScenario.wire,
    sharedKey,
  );
  const replacementInput = {
    ...realEnrollmentScenario.input,
    purpose: "REPLACEMENT_DEVICE",
  };
  const replacementContext = {
    received_at: "2026-07-29T10:00:01Z",
    subject_handle: "synthetic_replacement_route_scope_handle",
    nonce: "synthetic_replacement_route_scope_nonce",
  };
  const replacement = startEnrollment(
    state,
    replacementInput,
    replacementContext,
    sharedKey,
    primaryResponseCacheKeyring,
  );
  assert.equal(ordinary.outcome, "CHALLENGE_CREATED");
  assert.equal(replacement.outcome, "CHALLENGE_CREATED");
  assert.notEqual(
    ordinary.wire.subject_handle,
    replacement.wire.subject_handle,
    "distinct route scopes shared one enrollment response",
  );
  assert.equal(state.responseLedger.size, 2);
}
{
  const state = enrollmentStateFromFixture(executableSecurity);
  assert.equal(
    startEnrollment(
      state,
      {
        ...realEnrollmentScenario.input,
        request_id: "80000000-0000-4000-8000-000000000099",
      },
      enrollmentServerContextFromWire(realEnrollmentScenario.wire),
      syntheticUuid("enrollment-start:malformed-client-field"),
      primaryResponseCacheKeyring,
    ).outcome,
    "REJECT_MALFORMED",
  );
  assert.equal(state.challenges.size, 0);
  assert.equal(state.auditEvents.size, 0);
}
{
  const state = enrollmentStateFromFixture(executableSecurity);
  startEnrollmentFromFixture(
    state,
    realEnrollmentScenario.input,
    realEnrollmentScenario.wire,
  );
  const ownership =
    state.ownerships.get(
      "10000000-0000-4000-8000-000000000001:20000000-0000-4000-8000-000000000001",
    );
  ownership.status = "REVOKED";
  ownership.revoked_at = "2026-07-29T10:00:30Z";
  assert.equal(
    outcomeOf(completeEnrollment(
      state,
      enrollmentCompletionFor(realEnrollmentScenario.wire),
      "2026-07-29T10:01:00Z",
      "60000000-0000-4000-8000-000000000042",
      primaryResponseCacheKeyring,
    )),
    "REJECT_INACTIVE_OWNERSHIP",
  );
  assert.equal(state.devices.size, 0);
  assert.equal(state.sessions.size, 0);
  assert.equal(
    state.challenges.get(realEnrollmentScenario.wire.subject_handle).status,
    "CONSUMED",
  );
}
for (const forbiddenCompletionField of [
  "completed_at",
  "received_at",
  "request_id",
  "user_id",
]) {
  const state = enrollmentStateFromFixture(executableSecurity);
  startEnrollmentFromFixture(
    state,
    realEnrollmentScenario.input,
    realEnrollmentScenario.wire,
  );
  const completion = {
    ...enrollmentCompletionFor(realEnrollmentScenario.wire),
    [forbiddenCompletionField]: forbiddenCompletionField.endsWith("_at")
      ? "2026-07-29T10:00:59Z"
      : "10000000-0000-4000-8000-000000000099",
  };
  assert(!validatorFor("EnrollmentCompletionInput")(completion));
  assert.equal(
    outcomeOf(completeEnrollment(
      state,
      completion,
      "2026-07-29T10:01:00Z",
      syntheticUuid(`complete-forbidden:${forbiddenCompletionField}`),
      primaryResponseCacheKeyring,
    )),
    "REJECT_MALFORMED",
  );
  assert.equal(state.challenges.size, 1);
  assert.equal(state.devices.size, 0);
}
{
  const state = enrollmentStateFromFixture(executableSecurity);
  startEnrollmentFromFixture(
    state,
    realEnrollmentScenario.input,
    realEnrollmentScenario.wire,
  );
  const invalidBinding = enrollmentCompletionFor(realEnrollmentScenario.wire, {
    signature: `ed25519-signature:${"0".repeat(128)}`,
  });
  const invalidProofKey =
    "60000000-0000-4000-8000-000000000011";
  const invalidProofFirst = completeEnrollment(
      state,
      invalidBinding,
      "2026-07-29T10:01:00Z",
      invalidProofKey,
      primaryResponseCacheKeyring,
    );
  assert.equal(
    outcomeOf(invalidProofFirst),
    "CONSUMED_INVALID_PROOF",
  );
  assertDeepEqual(
    completeEnrollment(
      state,
      invalidBinding,
      "2026-07-29T10:01:01Z",
      invalidProofKey,
      replicaResponseCacheKeyring,
    ),
    invalidProofFirst,
    "invalid enrollment proof replay did not return its exact first result",
  );
  assert.equal(state.devices.size, 0);
  assert.equal(state.auditEvents.size, 2);
  assert.equal(state.outboxRecords.size, 2);
}
{
  const state = enrollmentStateFromFixture(executableSecurity);
  startEnrollmentFromFixture(
    state,
    realEnrollmentScenario.input,
    realEnrollmentScenario.wire,
  );
  assert.equal(
    outcomeOf(completeEnrollment(
      state,
      enrollmentCompletionFor(realEnrollmentScenario.wire),
      realEnrollmentScenario.wire.expires_at,
      "60000000-0000-4000-8000-000000000014",
      primaryResponseCacheKeyring,
    )),
    "REJECT_EXPIRED",
  );
  assert.equal(state.devices.size, 0);
  assert.equal(state.enrollments.size, 0);
  assert.equal(
    state.challenges.get(realEnrollmentScenario.wire.subject_handle).status,
    "EXPIRED",
  );
}
{
  const state = enrollmentStateFromFixture(executableSecurity);
  startEnrollmentFromFixture(
    state,
    realEnrollmentScenario.input,
    realEnrollmentScenario.wire,
  );
  const completion = enrollmentCompletionFor(realEnrollmentScenario.wire);
  const firstRequestKey = "60000000-0000-4000-8000-000000000012";
  const firstEnrollmentResponse = completeEnrollment(
    state,
    completion,
    "2026-07-29T10:01:00Z",
    firstRequestKey,
    primaryResponseCacheKeyring,
  );
  assert.equal(
    outcomeOf(firstEnrollmentResponse),
    "CONSUMED_SUCCESS",
  );
  const enrollmentRoute = enrollmentRouteTemplate(
    realEnrollmentScenario.input.purpose,
    "complete",
  );
  const enrollmentRequestDigest = authenticatedMutationDigest({
    routeTemplate: enrollmentRoute,
    body: completion,
    userId: "10000000-0000-4000-8000-000000000001",
    tradingAccountId: "20000000-0000-4000-8000-000000000001",
  });
  const enrollmentCacheContext = responseCacheContext({
    userId: "10000000-0000-4000-8000-000000000001",
    tradingAccountId: "20000000-0000-4000-8000-000000000001",
    routeTemplate: enrollmentRoute,
    requestKey: firstRequestKey,
    requestDigest: enrollmentRequestDigest,
  });
  const enrollmentCacheEntry = state.responseLedger.get(
    responseLedgerKey(enrollmentCacheContext),
  );
  assert(
    !JSON.stringify(enrollmentCacheEntry).includes("synthetic-access-token:") &&
      !JSON.stringify(enrollmentCacheEntry).includes(
        "synthetic-refresh-token:",
      ),
    "enrollment response cache persisted plaintext credential material",
  );
  const cachedEnrollmentResponse = openCachedResponse(
    enrollmentCacheEntry.response_cache,
    enrollmentCacheContext,
    "2026-07-29T10:01:01Z",
    restartedResponseCacheKeyring,
  );
  assert.equal(cachedEnrollmentResponse.outcome, "CONSUMED_SUCCESS");
  assert.equal(
    cachedEnrollmentResponse.replay_classification,
    "RETURN_RECORDED_RESULT",
  );
  assert.equal(typeof cachedEnrollmentResponse.device_id, "string");
  assert.equal(typeof cachedEnrollmentResponse.session_id, "string");
  assert.equal(typeof cachedEnrollmentResponse.refresh_family_id, "string");
  assertDeepEqual(
    completeEnrollment(
      state,
      completion,
      "2026-07-29T10:01:01Z",
      firstRequestKey,
      replicaResponseCacheKeyring,
    ),
    firstEnrollmentResponse,
    "enrollment retry did not recover the exact credential response",
  );
  assert.equal(
    outcomeOf(completeEnrollment(
      state,
      enrollmentCompletionFor(realEnrollmentScenario.wire, {
        signature: `ed25519-signature:${"0".repeat(128)}`,
      }),
      "2026-07-29T10:01:01Z",
      firstRequestKey,
      primaryResponseCacheKeyring,
    )),
    "IDEMPOTENCY_CONFLICT",
  );
  assert.equal(
    outcomeOf(completeEnrollment(
      state,
      completion,
      "2026-07-29T10:01:01Z",
      "60000000-0000-4000-8000-000000000013",
      primaryResponseCacheKeyring,
    )),
    "REJECT_REPLAY",
  );
  assert.equal(state.devices.size, 1);
  assert.equal(state.auditEvents.size, 3);
  assert.equal(state.outboxRecords.size, 4);
  assert.equal(
    outcomeOf(completeEnrollment(
      state,
      completion,
      "2026-07-29T10:03:00Z",
      firstRequestKey,
      primaryResponseCacheKeyring,
    )),
    "RECONCILIATION_REQUIRED_CACHE_EXPIRED",
  );
  assert.equal(state.devices.size, 1);
  assert.equal(
    state.responseLedger.get(responseLedgerKey(enrollmentCacheContext))
      .response_cache,
    undefined,
    "expired enrollment ciphertext was retained",
  );
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
  startEnrollmentFromFixture(
    state,
    realEnrollmentScenario.input,
    realEnrollmentScenario.wire,
  );
  assert.equal(
    outcomeOf(completeEnrollment(
      state,
      enrollmentCompletionFor(realEnrollmentScenario.wire),
      "2026-07-29T10:01:00Z",
      syntheticUuid(`complete:${expected}`),
      primaryResponseCacheKeyring,
    )),
    expected,
  );
  assert.equal(state.devices.size, 0);
  assert.equal(state.auditEvents.size, 2);
  assert.equal(state.outboxRecords.size, 2);
}
for (const purpose of ["ADDITIONAL_DEVICE", "REPLACEMENT_DEVICE"]) {
  const state = enrollmentStateFromFixture(executableSecurity);
  const priorDevice = clone(
    validFixtureSet.cases.find(({ schema }) => schema === "Device").value,
  );
  const priorSession = clone(
    validFixtureSet.cases.find(({ schema }) => schema === "Session").value,
  );
  const priorFamily = clone(
    validFixtureSet.cases.find(({ schema }) => schema === "RefreshFamilyState")
      .value,
  );
  priorFamily.family_created_at = "2026-07-22T10:00:00Z";
  priorFamily.family_deadline = "2026-08-21T10:00:00Z";
  priorFamily.tokens[0].family_created_at = priorFamily.family_created_at;
  priorFamily.tokens[0].family_deadline = priorFamily.family_deadline;
  priorFamily.tokens[0].issued_at = "2026-07-22T10:00:00Z";
  priorFamily.tokens[0].expires_at = "2026-07-29T10:00:00Z";
  priorFamily.tokens[1].family_created_at = priorFamily.family_created_at;
  priorFamily.tokens[1].family_deadline = priorFamily.family_deadline;
  priorFamily.tokens[1].issued_at = "2026-07-28T10:00:00Z";
  priorFamily.tokens[1].expires_at = "2026-08-04T10:00:00Z";
  state.devices.set(priorDevice.device_id, priorDevice);
  state.sessions.set(priorSession.session_id, priorSession);
  state.refreshFamilies.set(priorFamily.family_id, {
    user_id: priorDevice.user_id,
    state: priorFamily,
  });
  const { wire, completion } = signedSyntheticEnrollment(
    realEnrollmentScenario.wire,
    {
      purpose,
      subject_handle: `real_${purpose.toLowerCase()}_state_transition`,
    },
  );
  const input = {
    ...realEnrollmentScenario.input,
    purpose,
    candidate_public_key_fingerprint: wire.candidate_public_key_fingerprint,
  };
  startEnrollmentFromFixture(state, input, wire);
  assert.equal(
    outcomeOf(
      completeEnrollment(
        state,
        completion,
        "2026-07-29T10:01:00Z",
        syntheticUuid(`complete:${purpose}`),
        primaryResponseCacheKeyring,
      ),
    ),
    "CONSUMED_SUCCESS",
  );
  const priorShouldRemainActive = purpose === "ADDITIONAL_DEVICE";
  assert.equal(
    state.devices.get(priorDevice.device_id).status === "ACTIVE",
    priorShouldRemainActive,
  );
  assert.equal(
    state.sessions.get(priorSession.session_id).status === "ACTIVE",
    priorShouldRemainActive,
  );
  assert.equal(
    state.refreshFamilies.get(priorFamily.family_id).state.status === "ACTIVE",
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
  assert.equal(
    cases.length,
    2,
    `${route} needs unknown and wrong-password cases`,
  );
  assertDeepEqual(
    cases.map(({ status, envelope, timing_class, synthetic_challenge }) => ({
      status,
      envelope,
      timing_class,
      synthetic_challenge,
    })),
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
assert.equal(validWebSocketReauth.user_active, true);
assert.equal(validWebSocketReauth.account_ownership_active, true);
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
  "user_active",
  "account_ownership_active",
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
  assert.equal(
    accepted ? "ACCEPT" : "REJECT",
    scenario.expected,
    scenario.name,
  );
}
for (const scenario of executableSecurity.websocket_redaction_cases) {
  assert(
    security.websocket_reauthorization.redacted_fields.includes(scenario.field),
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
  security.websocket_reauthorization.reauthorization_binding_validation,
  "VALID_FRESH_ACCESS_TOKEN_AND_ACTIVE_USER_ACCOUNT_OWNERSHIP_DEVICE_SESSION_REFRESH_FAMILY_EXACT_MATCH_AT_VERIFICATION_TIME",
);
assert.equal(websocketProtocol.protocol_version, "fit.platform.websocket.v1");
assert.equal(
  websocketProtocol.phase0_openapi_relationship.status,
  "SUPERSEDED_FOR_PHASE_1_CONTROL_FRAMES_ONLY",
);
assertDeepEqual(
  websocketProtocol.client_control_messages.map(({ schema }) => schema),
  ["WebSocketReauth"],
);
assertDeepEqual(
  websocketProtocol.server_control_messages.map(({ schema }) => schema),
  ["WebSocketReauthRequired"],
);
assert.equal(
  websocketProtocol.phase0_openapi_relationship
    .phase0_file_remains_byte_identical,
  true,
);
assert.equal(
  websocketProtocol.phase0_openapi_relationship.consumer_source_of_truth,
  "THIS_MANIFEST_WITH_PHASE0_SCHEMA_FOR_APPLICATION_MESSAGES_AND_PLATFORM_SCHEMA_FOR_CONTROL_FRAMES",
);
assert.equal(
  websocketProtocol.application_messages.schema_source,
  "PHASE0_FIT_TRADE_V1_SCHEMA",
);
assertDeepEqual(
  websocketProtocol.application_messages.preserved_from_phase0,
  ["AuditEvent", "Operation", "PositionSnapshot"].map((name) => ({
    name,
    schema_ref: `../../jsonschema/fit-trade-v1.schema.json#/$defs/${name}`,
  })),
);
const websocketApplicationAjv = new Ajv2020({
  strict: true,
  strictRequired: false,
  strictTypes: false,
});
addFormats(websocketApplicationAjv);
websocketApplicationAjv.addSchema(phase0DomainSchema);
for (const { name, schema_ref: schemaRef } of websocketProtocol
  .application_messages.preserved_from_phase0) {
  assert(
    Object.hasOwn(phase0DomainSchema.$defs, name),
    `WebSocket application schema ${name} is absent from Phase 0`,
  );
  assert.equal(
    schemaRef,
    `../../jsonschema/fit-trade-v1.schema.json#/$defs/${name}`,
  );
  assert.equal(
    typeof websocketApplicationAjv.getSchema(
      `${phase0DomainSchema.$id}#/$defs/${name}`,
    ),
    "function",
    `WebSocket application schema ${name} did not compile from Phase 0`,
  );
}
assert(
  !Object.hasOwn(platformSchema.$defs, "Operation") &&
    !Object.hasOwn(platformSchema.$defs, "PositionSnapshot"),
  "WebSocket application messages must not resolve against platform-v1",
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
assertDeepEqual(security.request_digest.raw_request_target_decoder, {
  implementation_contract: "parseStrictRequestTarget",
  entry_point: "BEFORE_FRAMEWORK_ROUTING_AND_PARAMETER_BINDING",
  path_encoding: "EXACT_ASCII_NO_PERCENT_ENCODING",
  reject_path: [
    "fragment",
    "backslash",
    "empty_segment",
    "dot_segment",
    "percent_encoded_path_byte",
    "invalid_utf8",
  ],
  query_parameter_multiplicity: "EXACTLY_ZERO_OR_ONE_VALUE_PER_DECODED_KEY",
  reject_duplicate_decoded_query_keys: true,
  plus_is_literal_plus_not_space: true,
  route_query_schema: {
    "/v1/confirmations/{confirmation_id}/consume": "NO_QUERY_KEYS",
  },
});
assert.equal(
  security.request_digest.confirmation_route_template,
  "/v1/confirmations/{confirmation_id}/consume",
);
assert.equal(
  security.request_digest.request_envelope_validation_keyword,
  "x-fit-request-digest",
);
for (const scenario of executableSecurity.raw_request_target_cases) {
  let outcome = "REJECT";
  try {
    const decoded = parseStrictRequestTarget(scenario.raw_target);
    assert.equal(
      decoded.route_template,
      "/v1/confirmations/{confirmation_id}/consume",
    );
    assert.equal(
      decoded.path.confirmation_id,
      canonicalUuid(decoded.path.confirmation_id),
      `${scenario.name} did not produce a canonical lowercase UUID`,
    );
    outcome = "ACCEPT";
  } catch {
    outcome = "REJECT";
  }
  assert.equal(outcome, scenario.expected, scenario.name);
}
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
    if (schemaAccepted && scenario.entry_point === "HTTP_MUTATION_BODY") {
      const parsedDigest = canonicalRequestDigest({
        schema_version: "fit.platform.request-digest.v1",
        method: "POST",
        route_template: "/v1/confirmations/{confirmation_id}/consume",
        path: decoded.path,
        query: decoded.query,
        body: decoded.body,
        user_id: "10000000-0000-4000-8000-000000000001",
        trading_account_id: "20000000-0000-4000-8000-000000000001",
      });
      const plainDigest = canonicalRequestDigest({
        schema_version: "fit.platform.request-digest.v1",
        method: "POST",
        route_template: "/v1/confirmations/{confirmation_id}/consume",
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
    assertDeepEqual(
      attempts,
      [true, false],
      `${name} compare-and-set boundary`,
    );
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
    REPLACEMENT_ENROLLMENT: "REVOKE_EXISTING_AUTHENTICATED_SCOPE_BEFORE_CREATE",
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
    scenario.elapsed_ms <=
      security.revocation.maximum_propagation_seconds * 1000,
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
    workload_manifest_sha256: recoveryPolicy.workload.workload_manifest_sha256,
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
  failClosedWithNoFailedGate.restore.failure_reason_code = `${failureKind}_DETECTED`;
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
for (const malformed of [
  {},
  { schema_version: "fit.platform.recovery-evidence.v1" },
]) {
  assert.doesNotThrow(
    () => recoveryValidator(malformed),
    "RecoveryEvidence validator threw on malformed input",
  );
  assert(!recoveryValidator(malformed));
}

const validEventEnvelope = validFixtureSet.cases.find(
  ({ schema }) => schema === "EventEnvelope",
).value;
const validateEventPayload = validatorFor("EventPayload");
assert(
  validateEventPayload(validEventEnvelope.payload),
  `valid standalone EventPayload failed: ${ajv.errorsText(
    validateEventPayload.errors,
  )}`,
);
for (const mutate of [
  (value) => {
    value.event_kind = "WAL_ARCHIVE_INTERRUPTED";
  },
  (value) => {
    value.scope.trading_account_id = "20000000-0000-4000-8000-000000000099";
  },
  (value) => {
    value.data.record_id = "c0000000-0000-4000-8000-000000000099";
  },
]) {
  const standalonePayload = clone(validEventEnvelope.payload);
  mutate(standalonePayload);
  assert(
    !validateEventPayload(standalonePayload),
    "standalone EventPayload accepted record identity drift",
  );
}
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
for (const mutate of [
  (value) => {
    value.payload.data.record.occurred_at = "2026-07-29T10:00:02Z";
  },
  (value) => {
    value.payload.data.record.payload_digest = "d".repeat(64);
  },
]) {
  const adversarial = clone(validEventEnvelope);
  mutate(adversarial);
  if (adversarial.payload.data.record.payload_digest !== "d".repeat(64)) {
    adversarial.payload.data.record.payload_digest = durableRecordDigest(
      adversarial.payload.data.record,
    );
  }
  adversarial.payload_digest = sha256(jcsCanonicalize(adversarial.payload));
  assert(
    !validatorFor("EventEnvelope")(adversarial),
    "event envelope accepted durable record time or digest drift",
  );
}
for (const malformed of [
  {},
  { schema_version: "fit.platform.event-envelope.v1" },
]) {
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
  assert.equal(
    signedBytes.toString("hex"),
    vector.signed_bytes_hex,
    vector.name,
  );
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
  validFixtureSet.cases.find(({ schema }) => schema === "DeviceActionChallenge")
    .value,
  deviceActionProofVector.challenge,
);
const validEnrollmentCompletion = validFixtureSet.cases.find(
  ({ schema }) => schema === "EnrollmentCompletionInput",
).value;
assert.equal(
  validEnrollmentCompletion.candidate_public_key,
  golden.ed25519_proofs.public_key,
);
assert.equal(
  validEnrollmentCompletion.signature,
  enrollmentProofVector.signature,
);
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
    salt_bytes: Buffer.from(golden.argon2id_production.salt_hex, "hex").length,
    result_bytes: Buffer.from(golden.argon2id_production.result_hex, "hex")
      .length,
    version: golden.argon2id_production.version,
  },
  {
    profile_id: security.password_hashing.production_profile.profile_id,
    minimum_memory_kib:
      security.password_hashing.production_profile.minimum_memory_kib,
    minimum_iterations:
      security.password_hashing.production_profile.minimum_iterations,
    parallelism: security.password_hashing.production_profile.parallelism,
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
assertDeepEqual(nats.principals.map(({ name }) => name).sort(), [
  "internal-notification-consumer",
  "outbox-publisher",
  "platform-consumer",
]);
for (const principal of nats.principals) {
  assert(
    !(principal.stream_filter_subjects ?? []).some(
      (subject) => !exactSubjects.includes(subject),
    ),
    `${principal.name} has an unknown application filter`,
  );
  assert(
    !(principal.stream_filter_subjects ?? []).some(
      (subject) => subject.includes("*") || subject.includes(">"),
    ),
    `${principal.name} application filters must be exact`,
  );
  assert.equal(principal.jetstream_admin, false);
  assertDeepEqual(principal.identity_claims, []);
}
const publisher = nats.principals.find(
  ({ name }) => name === "outbox-publisher",
);
assertDeepEqual(publisher.publish, exactSubjects);
assertDeepEqual(publisher.subscribe, [
  "_INBOX.fit-platform.outbox-publisher.>",
]);
assertDeepEqual(publisher.jetstream_protocol, {
  publish_ack_inbox_prefix: "_INBOX.fit-platform.outbox-publisher.>",
  consumer_configuration: "NOT_ALLOWED",
});
for (const consumer of nats.principals.filter(
  ({ name }) => name !== "outbox-publisher",
)) {
  const durable =
    consumer.name === "platform-consumer"
      ? "PLATFORM_CONSUMER"
      : "INTERNAL_NOTIFICATION_CONSUMER";
  const inbox = `_INBOX.fit-platform.${consumer.name}.>`;
  const fetch = `$JS.API.CONSUMER.MSG.NEXT.FIT_PLATFORM_V1.${durable}`;
  const ack = `$JS.ACK.FIT_PLATFORM_V1.${durable}.>`;
  assertDeepEqual(consumer.publish, [fetch, ack]);
  assertDeepEqual(consumer.subscribe, [inbox]);
  assertDeepEqual(
    consumer.stream_filter_subjects,
    nats.subject_bindings
      .filter((binding) => binding.consumer === consumer.name)
      .map(({ subject }) => subject),
    `${consumer.name} subject allowlist drifted`,
  );
  assertDeepEqual(consumer.jetstream_protocol, {
    delivery_mode: "DURABLE_PULL",
    stream: "FIT_PLATFORM_V1",
    durable_consumer: durable,
    fetch_subject: fetch,
    reply_inbox_prefix: inbox,
    ack_subject_prefix: ack,
    consumer_configuration: "INFRASTRUCTURE_BOOTSTRAP_ONLY",
  });
}
assertDeepEqual(nats.wildcard_policy, {
  application_subjects: "FORBIDDEN",
  jetstream_protocol:
    "ONLY_THE_EXACT_PER_PRINCIPAL_INBOX_AND_ACK_PREFIXES_LISTED_ABOVE",
  broad_stream_or_consumer_admin: "FORBIDDEN",
});
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
const versionOneWithPredecessor = clone(causallyValidOutbox.event);
versionOneWithPredecessor.previous_event_id =
  "a0000000-0000-4000-8000-000000000099";
assert(
  !validateEventEnvelope(versionOneWithPredecessor),
  "aggregate version 1 accepted a predecessor",
);
const versionTwoWithoutPredecessor = clone(causallyValidOutbox.event);
versionTwoWithoutPredecessor.aggregate_version = 2;
versionTwoWithoutPredecessor.payload.event_identity.aggregate_version = 2;
versionTwoWithoutPredecessor.payload_digest = sha256(
  jcsCanonicalize(versionTwoWithoutPredecessor.payload),
);
assert(
  !validateEventEnvelope(versionTwoWithoutPredecessor),
  "aggregate version 2 accepted no predecessor",
);
const versionTwoWithPredecessor = clone(versionTwoWithoutPredecessor);
versionTwoWithPredecessor.previous_event_id =
  "a0000000-0000-4000-8000-000000000099";
versionTwoWithPredecessor.payload.event_identity.previous_event_id =
  versionTwoWithPredecessor.previous_event_id;
versionTwoWithPredecessor.payload_digest = sha256(
  jcsCanonicalize(versionTwoWithPredecessor.payload),
);
assert(
  validateEventEnvelope(versionTwoWithPredecessor),
  "aggregate version 2 rejected an explicit predecessor",
);
const selfPredecessor = clone(versionTwoWithPredecessor);
selfPredecessor.previous_event_id = selfPredecessor.event_id;
selfPredecessor.payload.event_identity.previous_event_id =
  selfPredecessor.event_id;
selfPredecessor.payload_digest = sha256(
  jcsCanonicalize(selfPredecessor.payload),
);
assert(
  !validateEventEnvelope(selfPredecessor),
  "event accepted itself as previous_event_id",
);
const unregisteredStandalonePayload = clone(
  causallyValidOutbox.event.payload,
);
unregisteredStandalonePayload.subject =
  "fit.platform.v1.unregistered.attacker-selected";
assert(
  !validatorFor("EventPayload")(unregisteredStandalonePayload),
  "standalone EventPayload accepted an unregistered subject",
);
const semanticallyWrongStandalonePayload = clone(
  golden.payload_digest.value,
);
semanticallyWrongStandalonePayload.subject =
  "fit.platform.v1.auth-security.device-enrolled";
assert(
  validatorFor("EventPayload")(golden.payload_digest.value),
  "golden standalone EventPayload must be a valid semantic baseline",
);
assert(
  !validatorFor("EventPayload")(semanticallyWrongStandalonePayload),
  "standalone EventPayload accepted a registered but wrong subject",
);
{
  const state = enrollmentStateFromFixture(executableSecurity);
  const record = clone(causallyValidOutbox.event.payload.data.record);
  record.event_id = "c0000000-0000-4000-8000-000000000002";
  record.payload_digest = durableRecordDigest(record);
  const aggregateId = causallyValidOutbox.event.aggregate_id;
  state.aggregateState.set(`ENROLLMENT:${aggregateId}`, {
    version: 1,
    lastEventId: causallyValidOutbox.event.event_id,
  });
  assert.throws(
    () =>
      persistEnrollmentOutbox(
        state,
        causallyValidOutbox.event.payload.data.record_type,
        record,
        causallyValidOutbox.event.subject,
        aggregateId,
        0,
      ),
    /expected-version compare-and-set conflict/u,
    "a stale aggregate producer created a fork",
  );
  assert.equal(state.outboxRecords.size, 0);
  const appended = persistEnrollmentOutbox(
    state,
    causallyValidOutbox.event.payload.data.record_type,
    record,
    causallyValidOutbox.event.subject,
    aggregateId,
    1,
  );
  assert.equal(appended.event.aggregate_version, 2);
  assert.equal(
    appended.event.previous_event_id,
    causallyValidOutbox.event.event_id,
  );
}
const futureEventOutbox = clone(causallyValidOutbox);
futureEventOutbox.event.occurred_at = "2026-07-30T10:00:00Z";
futureEventOutbox.event.payload.data.record.occurred_at =
  futureEventOutbox.event.occurred_at;
futureEventOutbox.event.payload.data.record.payload_digest =
  durableRecordDigest(futureEventOutbox.event.payload.data.record);
futureEventOutbox.event.payload_digest = sha256(
  jcsCanonicalize(futureEventOutbox.event.payload),
);
assert(
  validateEventEnvelope(futureEventOutbox.event),
  "future-event Outbox probe must isolate Outbox causality",
);
assert(
  !validateOutbox(futureEventOutbox),
  "Outbox accepted an event occurring after the Outbox record was created",
);
const aggregateHistoryFirst = clone(causallyValidOutbox.event);
const aggregateHistorySecond = clone(aggregateHistoryFirst);
aggregateHistorySecond.event_id = "c0000000-0000-4000-8000-000000000002";
aggregateHistorySecond.aggregate_version = 2;
aggregateHistorySecond.previous_event_id = aggregateHistoryFirst.event_id;
aggregateHistorySecond.occurred_at = "2026-07-29T10:00:02Z";
aggregateHistorySecond.payload.event_identity = {
  event_id: aggregateHistorySecond.event_id,
  aggregate_type: aggregateHistorySecond.aggregate_type,
  aggregate_id: aggregateHistorySecond.aggregate_id,
  aggregate_version: aggregateHistorySecond.aggregate_version,
  previous_event_id: aggregateHistorySecond.previous_event_id,
};
aggregateHistorySecond.payload.data.record_id =
  "c0000000-0000-4000-8000-000000000002";
aggregateHistorySecond.payload.data.record.event_id =
  aggregateHistorySecond.payload.data.record_id;
aggregateHistorySecond.payload.data.record.occurred_at =
  aggregateHistorySecond.occurred_at;
aggregateHistorySecond.payload.data.record.payload_digest = durableRecordDigest(
  aggregateHistorySecond.payload.data.record,
);
aggregateHistorySecond.payload_digest = sha256(
  jcsCanonicalize(aggregateHistorySecond.payload),
);
const aggregateHistory = {
  schema_version: "fit.platform.aggregate-event-history.v1",
  aggregate_type: aggregateHistoryFirst.aggregate_type,
  aggregate_id: aggregateHistoryFirst.aggregate_id,
  starting_version: 1,
  events: [aggregateHistoryFirst, aggregateHistorySecond],
};
const validateAggregateHistory = validatorFor("AggregateEventHistory");
assert(
  validateAggregateHistory(aggregateHistory),
  `valid aggregate history failed: ${ajv.errorsText(
    validateAggregateHistory.errors,
  )}`,
);
function rebindHistoryEventPayload(event) {
  const record = event.payload.data.record;
  event.payload.event_identity = {
    event_id: event.event_id,
    aggregate_type: event.aggregate_type,
    aggregate_id: event.aggregate_id,
    aggregate_version: event.aggregate_version,
    ...(Object.hasOwn(event, "previous_event_id")
      ? { previous_event_id: event.previous_event_id }
      : {}),
  };
  event.payload.data.record_id = event.event_id;
  if (event.payload.data.record_type === "audit") {
    record.event_id = event.event_id;
  } else {
    record.notification_id = event.event_id;
  }
  record.occurred_at = event.occurred_at;
  record.correlation_id = event.correlation_id;
  record.payload_digest = durableRecordDigest(record);
  event.payload_digest = sha256(jcsCanonicalize(event.payload));
}
for (const mutate of [
  (value) => {
    value.starting_version = 2;
  },
  (value) => {
    value.events[1].aggregate_version = 3;
    rebindHistoryEventPayload(value.events[1]);
  },
  (value) => {
    value.events[1].previous_event_id = "a0000000-0000-4000-8000-000000000099";
    rebindHistoryEventPayload(value.events[1]);
  },
  (value) => {
    value.events[1].occurred_at = "2026-07-29T09:59:59Z";
    rebindHistoryEventPayload(value.events[1]);
  },
  (value) => {
    value.events[1].correlation_id = "b0000000-0000-4000-8000-000000000099";
    rebindHistoryEventPayload(value.events[1]);
  },
]) {
  const invalidHistory = clone(aggregateHistory);
  mutate(invalidHistory);
  assert(
    invalidHistory.events.every((event) => validateEventEnvelope(event)),
    "aggregate-history negative was rejected by an individual envelope first",
  );
  assert(
    !validateAggregateHistory(invalidHistory),
    "aggregate history accepted duplicate, gap, reversal, or broken predecessor",
  );
}
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
  };
  auditEvent.payload_digest = durableRecordDigest(auditEvent);
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
    };
    notification.payload_digest = durableRecordDigest(notification);
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
    const eventId = durable.recordId;
    const previousEventId =
      recordIndex === 0 ? null : durableRecords[recordIndex - 1].recordId;
    const payload = {
      schema_version: "fit.platform.event-payload.v1",
      subject: durable.subject,
      event_kind: chain.kind,
      scope: chain.scope,
      event_identity: {
        event_id: eventId,
        aggregate_type: chain.aggregate_type,
        aggregate_id: chain.aggregate_id,
        aggregate_version: recordIndex + 1,
        ...(previousEventId === null
          ? {}
          : { previous_event_id: previousEventId }),
      },
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
      event_id: eventId,
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
      ...(previousEventId === null
        ? {}
        : { previous_event_id: previousEventId }),
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
  assert(
    validatorFor("AggregateEventHistory")({
      schema_version: "fit.platform.aggregate-event-history.v1",
      aggregate_type: chain.aggregate_type,
      aggregate_id: chain.aggregate_id,
      starting_version: 1,
      events: outboxRecords.map(({ event }) => event),
    }),
    `${chain.name} aggregate history invalid`,
  );
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
      ...(["DEVICE_ENROLLED", "DEVICE_REPLACED", "DEVICE_REVOKED"].includes(
        binding.event_kind,
      )
        ? { device_id: "30000000-0000-4000-8000-000000000001" }
        : {}),
      ...(["SESSION_REVOKED", "REFRESH_REUSE_DETECTED"].includes(
        binding.event_kind,
      )
        ? { session_id: "40000000-0000-4000-8000-000000000001" }
        : {}),
      occurred_at: "2026-07-29T10:00:01Z",
    };
  }
  durableRecord.payload_digest = durableRecordDigest(durableRecord);
  const eventId = recordId;
  const aggregateType = "SYNTHETIC_AGGREGATE";
  const aggregateId = "16000000-0000-4000-8000-000000000001";
  const payload = {
    schema_version: "fit.platform.event-payload.v1",
    subject: binding.subject,
    event_kind: binding.event_kind,
    scope,
    event_identity: {
      event_id: eventId,
      aggregate_type: aggregateType,
      aggregate_id: aggregateId,
      aggregate_version: 1,
    },
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
    event_id: eventId,
    event_kind: binding.event_kind,
    scope,
    aggregate_type: aggregateType,
    aggregate_id: aggregateId,
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
assertDeepEqual(transactions.response_cache_security, {
  algorithm: "AES-256-GCM",
  ttl_seconds: 120,
  key_source:
    "DEPLOYMENT_SECRET_INJECTION_SHARED_ACROSS_REPLICAS_AND_RESTARTS_NOT_POSTGRESQL",
  key_rotation_identifier_authenticated: true,
  authenticated_context: [
    "user_id",
    "trading_account_id",
    "route_template",
    "idempotency_key",
    "canonical_request_digest",
    "created_at_ms",
    "expires_at_ms",
  ],
  expired_ciphertext:
    "ERASE_ON_REQUEST_AND_PROACTIVE_BACKGROUND_SWEEP_KEEP_ONLY_NONSECRET_RECONCILIATION_MARKER",
  replica_and_restart_recovery:
    "SAME_DEPLOYMENT_KEYRING_CAN_DECRYPT_UNEXPIRED_CACHE",
});
for (const workflow of transactions.workflows) {
  const statements = workflow.single_postgresql_transaction ?? [];
  if (
    failureInjection.transaction_cut_rule.commit_response_flags.some(
      (flag) => workflow[flag] === true,
    )
  ) {
    assert(
      Array.isArray(workflow.route_templates) &&
        workflow.route_templates.length > 0,
      `${workflow.name} lacks an authenticated route template`,
    );
    const claimIndex = statements.findIndex((statement) =>
      /^claim_scoped_(?:idempotency_key|request_id)_and_canonical_request_digest$/u.test(
        statement,
      ),
    );
    assert(claimIndex >= 0, `${workflow.name} lacks a scoped request claim`);
    const authorityResolutionIndex = statements.findLastIndex(
      (statement, index) =>
        index < claimIndex &&
        /(?:resolve_(?:server_owned_trading_account|real_challenge_owner_scope|refresh_family_owner_scope)|assert_resolved_user|verify_(?:active_)?owner|verify_owner_scope)/u.test(
          statement,
        ),
    );
    const firstDurableEffectIndex = statements.findIndex((statement) =>
      /^(?:record|set|create|revoke|rotate|write|persist_(?!exact)|consume)/u.test(
        statement,
      ),
    );
    assert(
      firstDurableEffectIndex < 0 || claimIndex < firstDurableEffectIndex,
      `${workflow.name} claims idempotency after a durable effect`,
    );
    if (
      statements.some((statement) =>
        /(?:resolve_(?:server_owned_trading_account|real_challenge_owner_scope|refresh_family_owner_scope)|assert_resolved_user|verify_(?:active_)?owner|verify_owner_scope)/u.test(
          statement,
        ),
      )
    ) {
      assert(
        authorityResolutionIndex >= 0 && authorityResolutionIndex < claimIndex,
        `${workflow.name} claims a user-scoped key before server-owned authority resolution`,
      );
    }
    const exactResponseIndex = statements.findIndex((statement) =>
      /persist_exact_.*(?:result|response).*same_transaction/u.test(statement),
    );
    assert(
      exactResponseIndex === statements.length - 1,
      `${workflow.name} does not atomically persist its exact response last`,
    );
  }
  const outboxWriteIndex = statements.findIndex((statement) =>
    /(?:write|persist).*outbox/u.test(statement),
  );
  if (outboxWriteIndex >= 0) {
    const aggregateLockIndex = statements.findIndex(
      (statement) =>
        /lock_.*aggregate_append_state.*compare_expected_version/u.test(
          statement,
        ),
    );
    assert(
      aggregateLockIndex >= 0 && aggregateLockIndex < outboxWriteIndex,
      `${workflow.name} writes Outbox without an earlier expected-version CAS`,
    );
  }
}
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
    platformSchema.$defs.EventSubject.enum.includes(eventContract.subject),
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
    passwordFailureWorkflow.single_postgresql_transaction.includes(
      requiredStep,
    ),
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
  ack_loss: "REDELIVERY_MUST_HIT_INBOX_AND_CREATE_NO_DUPLICATE_BUSINESS_EFFECT",
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
    return ["AFTER_POSTGRESQL_COMMIT_BEFORE_NATS_ACK", "AFTER_NATS_ACK"];
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

function executeStatementTransaction(
  statements,
  abortAfterStatement = null,
  failureMode = "TRANSACTION_ABORT",
) {
  assert(
    ["TRANSACTION_ABORT", "PROCESS_KILL"].includes(failureMode),
    "unknown transaction failure mode",
  );
  const durable = {
    effects: [],
    statement_rows: new Map(),
    sequence: 0,
  };
  const staged = structuredClone(durable);
  try {
    if (abortAfterStatement === 0) {
      throw new Error(`${failureMode}_BEFORE_FIRST_STATEMENT`);
    }
    for (const [index, statement] of statements.entries()) {
      staged.sequence += 1;
      staged.statement_rows.set(
        sha256(`${staged.sequence}:${statement}`),
        { sequence: staged.sequence, statement },
      );
      staged.effects.push(statement);
      if (abortAfterStatement === index + 1) {
        throw new Error(`${failureMode}_AFTER_STATEMENT_${index + 1}`);
      }
    }
  } catch {
    return durable;
  }
  return staged;
}

function executeIdempotentResponseLoss(runtime, request, keyring) {
  assert(keyring instanceof Map, "response-cache keyring must be injected");
  const context = responseCacheContext({
    userId: request.user_id,
    tradingAccountId: request.trading_account_id,
    routeTemplate: request.route_template,
    requestKey: request.key,
    requestDigest: request.digest,
  });
  const ledgerKey = responseLedgerKey(context);
  const prior = runtime.ledger.get(ledgerKey);
  if (prior !== undefined) {
    if (prior.digest !== request.digest) return "IDEMPOTENCY_CONFLICT";
    if (
      prior.response_cache === undefined ||
      request.now_ms >= prior.response_cache.expires_at_ms
    ) {
      expireCachedResponse(prior, request.now_ms);
      return "RECONCILIATION_REQUIRED_CACHE_EXPIRED";
    }
    return openCachedResponse(
      prior.response_cache,
      context,
      request.now_ms,
      keyring,
    );
  }
  const staged = structuredClone(runtime);
  staged.effects.push(request.effect);
  const response = {
    replay_classification: "RETURN_RECORDED_RESULT",
    outcome: "COMMITTED",
    effect_receipt: sha256(request.effect),
  };
  staged.ledger.set(ledgerKey, {
    digest: request.digest,
    response_cache: sealCachedResponse(
      response,
      context,
      request.now_ms,
      keyring,
    ),
  });
  if (request.abort_after_effect === true) {
    return "SIMULATED_TRANSACTION_ABORT";
  }
  replaceRuntimeState(runtime, staged);
  return request.response_lost
    ? "UNKNOWN_REQUIRES_RECONCILIATION"
    : response;
}

const realRollbackProbe = executeStatementTransaction(
  ["insert_inbox", "write_business_effect", "mark_inbox_applied"],
  2,
);
assertDeepEqual(realRollbackProbe.effects, []);
assert.equal(realRollbackProbe.statement_rows.size, 0);
assert.equal(realRollbackProbe.sequence, 0);
const realCommitProbe = executeStatementTransaction([
  "insert_inbox",
  "write_business_effect",
  "mark_inbox_applied",
]);
assert.equal(realCommitProbe.statement_rows.size, 3);
assert.equal(realCommitProbe.sequence, 3);

let executedTransactionCutCount = 0;
let executedProcessKillCutCount = 0;
let executedResponseLossCount = 0;
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
      Array.isArray(workflow.response_steps) &&
        workflow.response_steps.length > 0,
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
    assertDeepEqual(
      executeStatementTransaction(
        statements.map(({ statement }) => statement),
        0,
      ).effects,
      [],
      `${workflow.name} wrote before its first statement`,
    );
    executedTransactionCutCount += 1;
    failureCutEvidence.push(
      `${workflow.name}:${transactionName}:PROCESS_KILL_BEFORE_FIRST_STATEMENT`,
    );
    assertDeepEqual(
      executeStatementTransaction(
        statements.map(({ statement }) => statement),
        0,
        "PROCESS_KILL",
      ).effects,
      [],
      `${workflow.name} process kill wrote before its first statement`,
    );
    executedProcessKillCutCount += 1;
    for (const [statementIndex, { field, statement }] of statements.entries()) {
      assert.equal(typeof statement, "string", `${workflow.name}.${field}`);
      failureCutEvidence.push(
        `${workflow.name}:${transactionName}:AFTER_STATEMENT_${statementIndex + 1}:${field}:${statement}`,
      );
      assertDeepEqual(
        executeStatementTransaction(
          statements.map((entry) => entry.statement),
          statementIndex + 1,
        ).effects,
        [],
        `${workflow.name} leaked a partial effect after statement ${statementIndex + 1}`,
      );
      executedTransactionCutCount += 1;
      failureCutEvidence.push(
        `${workflow.name}:${transactionName}:PROCESS_KILL_AFTER_STATEMENT_${statementIndex + 1}:${field}:${statement}`,
      );
      assertDeepEqual(
        executeStatementTransaction(
          statements.map((entry) => entry.statement),
          statementIndex + 1,
          "PROCESS_KILL",
        ).effects,
        [],
        `${workflow.name} process kill leaked a partial effect after statement ${statementIndex + 1}`,
      );
      executedProcessKillCutCount += 1;
    }
    failureCutEvidence.push(`${workflow.name}:${transactionName}:AFTER_COMMIT`);
    assertDeepEqual(
      executeStatementTransaction(statements.map(({ statement }) => statement))
        .effects,
      statements.map(({ statement }) => statement),
      `${workflow.name} committed an incomplete statement set`,
    );
    executedTransactionCutCount += 1;
    const conditionalStatements = statements.filter(({ statement }) =>
      /(?:_if_|if_present|only_if)/u.test(statement),
    );
    if (conditionalStatements.length > 0) {
      const absentPathStatements = statements
        .filter(
          ({ statement }) => !/(?:_if_|if_present|only_if)/u.test(statement),
        )
        .map(({ statement }) => statement);
      assertDeepEqual(
        executeStatementTransaction(absentPathStatements).effects,
        absentPathStatements,
        `${workflow.name} conditional-absent transaction drifted`,
      );
      executedTransactionCutCount += 1;
    }
  }
  if (
    failureInjection.transaction_cut_rule.commit_response_flags.some(
      (flag) => workflow[flag] === true,
    )
  ) {
    failureCutEvidence.push(
      `${workflow.name}:RESPONSE:AFTER_COMMIT_BEFORE_RESPONSE:UNKNOWN_REQUIRES_RECONCILIATION`,
    );
    assert(
      ["idempotency_key", "request_id"].includes(workflow.stable_request_key),
      `${workflow.name} lacks a stable response-loss key`,
    );
    assert.equal(
      typeof workflow.same_key_same_digest_after_response_loss,
      "string",
      `${workflow.name} lacks same-key reconciliation semantics`,
    );
    assert.equal(
      workflow.same_key_different_digest,
      "IDEMPOTENCY_CONFLICT",
      `${workflow.name} lacks changed-request conflict semantics`,
    );
    const runtime = { effects: [], ledger: new Map() };
    const request = {
      key: "60000000-0000-4000-8000-000000000031",
      user_id: "10000000-0000-4000-8000-000000000001",
      trading_account_id: "20000000-0000-4000-8000-000000000001",
      route_template: workflow.route_templates[0],
      effect: `${workflow.name}:COMPLETE_DURABLE_EFFECT`,
      response_lost: true,
      now_ms: 1_000,
    };
    request.body = {
      workflow: workflow.name,
      requested_effect: request.effect,
    };
    request.digest = authenticatedMutationDigest({
      routeTemplate: request.route_template,
      body: request.body,
      userId: request.user_id,
      tradingAccountId: request.trading_account_id,
    });
    const abortedRuntime = { effects: [], ledger: new Map() };
    assert.equal(
      executeIdempotentResponseLoss(
        abortedRuntime,
        {
          ...request,
          abort_after_effect: true,
        },
        primaryResponseCacheKeyring,
      ),
      "SIMULATED_TRANSACTION_ABORT",
      workflow.name,
    );
    assertDeepEqual(abortedRuntime.effects, [], workflow.name);
    assert.equal(abortedRuntime.ledger.size, 0, workflow.name);
    assert.equal(
      executeIdempotentResponseLoss(
        runtime,
        request,
        primaryResponseCacheKeyring,
      ),
      "UNKNOWN_REQUIRES_RECONCILIATION",
      workflow.name,
    );
    const responseLossContext = responseCacheContext({
      userId: request.user_id,
      tradingAccountId: request.trading_account_id,
      routeTemplate: request.route_template,
      requestKey: request.key,
      requestDigest: request.digest,
    });
    const responseLossLedgerEntry = runtime.ledger.get(
      responseLedgerKey(responseLossContext),
    );
    assert(
      !JSON.stringify(responseLossLedgerEntry).includes(request.effect),
      `${workflow.name} cached plaintext response material`,
    );
    assertDeepEqual(
      openCachedResponse(
        responseLossLedgerEntry.response_cache,
        responseLossContext,
        1_001,
        restartedResponseCacheKeyring,
      ),
      {
        effect_receipt: sha256(request.effect),
        outcome: "COMMITTED",
        replay_classification: "RETURN_RECORDED_RESULT",
      },
      `${workflow.name} cannot recover its exact committed response`,
    );
    const proactivelySweptLedger = structuredClone(runtime.ledger);
    sweepExpiredResponseCaches(proactivelySweptLedger, 121_000);
    const proactivelySweptEntry = proactivelySweptLedger.get(
      responseLedgerKey(responseLossContext),
    );
    assert.equal(
      proactivelySweptEntry.response_cache,
      undefined,
      `${workflow.name} proactive sweep retained expired ciphertext`,
    );
    assert.equal(
      proactivelySweptEntry.cache_expired_at_ms,
      121_000,
      `${workflow.name} proactive sweep omitted its nonsecret marker`,
    );
    assertDeepEqual(
      executeIdempotentResponseLoss(
        runtime,
        {
          ...request,
          response_lost: false,
          now_ms: 1_001,
        },
        replicaResponseCacheKeyring,
      ),
      {
        effect_receipt: sha256(request.effect),
        outcome: "COMMITTED",
        replay_classification: "RETURN_RECORDED_RESULT",
      },
      workflow.name,
    );
    assertDeepEqual(runtime.effects, [request.effect], workflow.name);
    assert.equal(
      executeIdempotentResponseLoss(
        runtime,
        {
          ...request,
          response_lost: false,
          now_ms: 121_000,
        },
        restartedResponseCacheKeyring,
      ),
      "RECONCILIATION_REQUIRED_CACHE_EXPIRED",
      workflow.name,
    );
    assertDeepEqual(runtime.effects, [request.effect], workflow.name);
    assert.equal(
      runtime.ledger.get(responseLedgerKey(responseLossContext))
        .response_cache,
      undefined,
      `${workflow.name} retained expired response ciphertext`,
    );
    assert.equal(
      executeIdempotentResponseLoss(
        runtime,
        {
          ...request,
          body: { ...request.body, requested_effect: `${request.effect}:CHANGED` },
          digest: authenticatedMutationDigest({
            routeTemplate: request.route_template,
            body: {
              ...request.body,
              requested_effect: `${request.effect}:CHANGED`,
            },
            userId: request.user_id,
            tradingAccountId: request.trading_account_id,
          }),
          response_lost: false,
          now_ms: 1_002,
        },
        primaryResponseCacheKeyring,
      ),
      "IDEMPOTENCY_CONFLICT",
      workflow.name,
    );
    assertDeepEqual(runtime.effects, [request.effect], workflow.name);
    if (Object.hasOwn(workflow, "response_cache")) {
      assert.equal(
        workflow.response_cache,
        "AEAD_CIPHERTEXT_ONLY_SHARED_RUNTIME_KEYRING_NOT_DATABASE_TTL_120_SECONDS",
        workflow.name,
      );
      assert.equal(
        workflow.response_cache_expired_same_key,
        "RECONCILIATION_REQUIRED_NO_REEXECUTION",
        workflow.name,
      );
    }
    executedResponseLossCount += 1;
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
assertDeepEqual(failureInjection.executable_adversarial_probes, {
  transaction_abort: "REAL_STAGED_MAP_AND_SEQUENCE_MUTATIONS_ROLL_BACK",
  process_kill:
    "DISCARD_PROCESS_LOCAL_STAGED_STATE_BEFORE_ATOMIC_COMMIT_VISIBILITY",
  network_partition_before_consumer_commit:
    "NO_INBOX_PROJECTION_OR_BUSINESS_EFFECT",
  broker_ack_loss_after_consumer_commit:
    "REDELIVERY_RETURNS_EXISTING_INBOX_RESULT_WITH_NO_DUPLICATE_EFFECT",
  same_event_id_changed_digest: "REJECT_AND_ALERT_WITH_NO_MUTATION",
  wrong_owner_scope: "REJECT_BEFORE_INBOX_OR_BUSINESS_EFFECT",
  aggregate_projection:
    "COMPARE_VERSION_AND_PREDECESSOR_IN_THE_INBOX_TRANSACTION",
  outbox_publish_boundaries:
    "EXECUTE_CLAIM_PUBLISH_ACK_MARK_WITH_STABLE_EVENT_ID_AND_PAYLOAD_DIGEST",
});
function executeInboxDelivery(
  runtime,
  event,
  expectedScope,
  failurePoint = null,
) {
  if (!validateEventEnvelope(event)) return "REJECT_INVALID_EVENT";
  if (jcsCanonicalize(event.scope) !== jcsCanonicalize(expectedScope)) {
    return "REJECT_WRONG_OWNER_SCOPE";
  }
  const incomingIdentity = {
    event_payload_digest: event.payload_digest,
    event_subject: event.subject,
    event_kind: event.event_kind,
    event_scope: event.scope,
    aggregate_type: event.aggregate_type,
    aggregate_id: event.aggregate_id,
    aggregate_version: event.aggregate_version,
    ...(Object.hasOwn(event, "previous_event_id")
      ? { previous_event_id: event.previous_event_id }
      : {}),
  };
  const inboxKey = `${runtime.consumer}:${event.event_id}`;
  const prior = runtime.inbox.get(inboxKey);
  if (prior !== undefined) {
    const priorIdentity = Object.fromEntries(
      Object.keys(incomingIdentity).map((field) => [field, prior[field]]),
    );
    return jcsCanonicalize(priorIdentity) ===
      jcsCanonicalize(incomingIdentity)
      ? "ACK_EXISTING_INBOX_RESULT"
      : "REJECT_AND_ALERT_CHANGED_EVENT_IDENTITY";
  }
  const aggregateKey = `${event.aggregate_type}:${event.aggregate_id}`;
  const priorAggregate = runtime.aggregateProjection.get(aggregateKey);
  const expectedVersion = priorAggregate?.version + 1 || 1;
  const expectedPreviousId = priorAggregate?.event_id;
  if (
    event.aggregate_version !== expectedVersion ||
    (expectedVersion === 1
      ? Object.hasOwn(event, "previous_event_id")
      : event.previous_event_id !== expectedPreviousId)
  ) {
    return "REJECT_AGGREGATE_ORDER";
  }
  const staged = structuredClone(runtime);
  const inboxRecord = {
    schema_version: "fit.platform.inbox-record.v1",
    consumer: runtime.consumer,
    event_id: event.event_id,
    ...incomingIdentity,
    received_at: "2026-07-29T10:00:02Z",
    business_transaction_id: syntheticUuid(
      `inbox:${runtime.consumer}:${event.event_id}`,
    ),
    state: "APPLIED",
  };
  inboxRecord.status = inboxRecord.state;
  delete inboxRecord.state;
  assert(validatorFor("InboxRecord")(inboxRecord));
  staged.inbox.set(inboxKey, inboxRecord);
  staged.aggregateProjection.set(aggregateKey, {
    version: event.aggregate_version,
    event_id: event.event_id,
  });
  staged.effects.push({
    event_id: event.event_id,
    payload_digest: event.payload_digest,
  });
  if (failurePoint === "NETWORK_PARTITION_BEFORE_COMMIT") {
    return "NETWORK_PARTITION_NO_COMMIT";
  }
  replaceRuntimeState(runtime, staged);
  return failurePoint === "BROKER_ACK_LOSS_AFTER_COMMIT"
    ? "COMMITTED_ACK_LOST"
    : "ACK";
}

const inboxRuntime = {
  consumer: "platform-consumer",
  inbox: new Map(),
  aggregateProjection: new Map(),
  effects: [],
};
const firstInboxEvent = clone(aggregateHistory.events[0]);
const secondInboxEvent = clone(aggregateHistory.events[1]);
const partitionRuntime = structuredClone(inboxRuntime);
assert.equal(
  executeInboxDelivery(
    partitionRuntime,
    firstInboxEvent,
    firstInboxEvent.scope,
    "NETWORK_PARTITION_BEFORE_COMMIT",
  ),
  "NETWORK_PARTITION_NO_COMMIT",
);
assert.equal(partitionRuntime.inbox.size, 0);
assert.equal(partitionRuntime.aggregateProjection.size, 0);
assertDeepEqual(partitionRuntime.effects, []);
assert.equal(
  executeInboxDelivery(
    inboxRuntime,
    firstInboxEvent,
    firstInboxEvent.scope,
    "BROKER_ACK_LOSS_AFTER_COMMIT",
  ),
  "COMMITTED_ACK_LOST",
);
assert.equal(
  executeInboxDelivery(inboxRuntime, firstInboxEvent, firstInboxEvent.scope),
  "ACK_EXISTING_INBOX_RESULT",
);
assert.equal(inboxRuntime.effects.length, 1);
const changedDigestRedelivery = clone(firstInboxEvent);
changedDigestRedelivery.payload.data.record.actor.id = "changed-auth-service";
changedDigestRedelivery.payload.data.record.payload_digest =
  durableRecordDigest(changedDigestRedelivery.payload.data.record);
changedDigestRedelivery.payload_digest = sha256(
  jcsCanonicalize(changedDigestRedelivery.payload),
);
assert.equal(
  executeInboxDelivery(
    inboxRuntime,
    changedDigestRedelivery,
    firstInboxEvent.scope,
  ),
  "REJECT_AND_ALERT_CHANGED_EVENT_IDENTITY",
);
assert.equal(inboxRuntime.effects.length, 1);
const priorInboxRecord = inboxRuntime.inbox.get(
  `${inboxRuntime.consumer}:${firstInboxEvent.event_id}`,
);
assert.notEqual(
  priorInboxRecord.event_payload_digest,
  changedDigestRedelivery.payload_digest,
);
assert.equal(
  executeInboxDelivery(inboxRuntime, firstInboxEvent, {
    type: "OWNER",
    user_id: "10000000-0000-4000-8000-000000000099",
    trading_account_id: "20000000-0000-4000-8000-000000000099",
  }),
  "REJECT_WRONG_OWNER_SCOPE",
  "prior inbox deduplication bypassed current owner-scope validation",
);
const wrongOwnerRuntime = {
  consumer: "platform-consumer",
  inbox: new Map(),
  aggregateProjection: new Map(),
  effects: [],
};
assert.equal(
  executeInboxDelivery(wrongOwnerRuntime, firstInboxEvent, {
    type: "OWNER",
    user_id: "10000000-0000-4000-8000-000000000099",
    trading_account_id: "20000000-0000-4000-8000-000000000099",
  }),
  "REJECT_WRONG_OWNER_SCOPE",
);
assert.equal(wrongOwnerRuntime.inbox.size, 0);
assertDeepEqual(wrongOwnerRuntime.effects, []);
assert.equal(
  executeInboxDelivery(inboxRuntime, secondInboxEvent, secondInboxEvent.scope),
  "ACK",
);
assert.equal(inboxRuntime.effects.length, 2);
function executeOutboxPublish(
  runtime,
  cutAfter,
  worker = "outbox-publisher",
  now = "2026-07-29T10:00:02Z",
) {
  const record = runtime.outbox;
  if (record.publication_state === "PUBLISHED") {
    return "ALREADY_PUBLISHED";
  }
  const nowMs = isoMilliseconds(now);
  if (
    record.publication_state === "CLAIMED" &&
    record.lease_owner !== worker &&
    isoMilliseconds(record.lease_expires_at) > nowMs
  ) {
    return "LEASE_HELD_BY_OTHER_WORKER";
  }
  record.publication_state = "CLAIMED";
  record.lease_owner = worker;
  record.lease_expires_at = new Date(nowMs + 30_000).toISOString();
  assert(validateOutbox(record), "claimed Outbox must be schema-valid");
  if (cutAfter === "CLAIM_COMMIT") return "CLAIMED_NOT_PUBLISHED";
  runtime.brokerDeliveries.push({
    event_id: record.event.event_id,
    payload_digest: record.event.payload_digest,
  });
  if (cutAfter === "PUBLISH_BEFORE_ACK") return "PUBLISHED_ACK_UNKNOWN";
  runtime.acknowledged.add(record.event.event_id);
  if (cutAfter === "ACK_BEFORE_MARK") return "ACKED_DATABASE_MARK_PENDING";
  record.publication_state = "PUBLISHED";
  record.published_at = new Date(nowMs + 1_000).toISOString();
  delete record.lease_owner;
  delete record.lease_expires_at;
  assert(validateOutbox(record), "published Outbox must be schema-valid");
  return "MARKED_PUBLISHED";
}
const pendingOutboxForPublish = clone(causallyValidOutbox);
pendingOutboxForPublish.publication_state = "PENDING";
delete pendingOutboxForPublish.published_at;
const outboxPublishRuntime = {
  outbox: pendingOutboxForPublish,
  brokerDeliveries: [],
  acknowledged: new Set(),
};
assert.equal(
  executeOutboxPublish(outboxPublishRuntime, "CLAIM_COMMIT"),
  "CLAIMED_NOT_PUBLISHED",
);
assert.equal(
  executeOutboxPublish(
    outboxPublishRuntime,
    "CLAIM_COMMIT",
    "outbox-publisher-replica",
    "2026-07-29T10:00:03Z",
  ),
  "LEASE_HELD_BY_OTHER_WORKER",
);
assertDeepEqual(outboxPublishRuntime.brokerDeliveries, []);
assert.equal(
  executeOutboxPublish(outboxPublishRuntime, "PUBLISH_BEFORE_ACK"),
  "PUBLISHED_ACK_UNKNOWN",
);
assert.equal(outboxPublishRuntime.brokerDeliveries.length, 1);
assert.equal(
  executeOutboxPublish(outboxPublishRuntime, "ACK_BEFORE_MARK"),
  "ACKED_DATABASE_MARK_PENDING",
);
assert.equal(outboxPublishRuntime.brokerDeliveries.length, 2);
assert(
  outboxPublishRuntime.brokerDeliveries.every(
    (delivery) =>
      delivery.event_id === causallyValidOutbox.event.event_id &&
      delivery.payload_digest === causallyValidOutbox.event.payload_digest,
  ),
  "outbox retry changed publish identity",
);
assert.equal(
  executeOutboxPublish(outboxPublishRuntime, "MARK"),
  "MARKED_PUBLISHED",
);
assert.equal(outboxPublishRuntime.outbox.publication_state, "PUBLISHED");
assert.equal(
  executeOutboxPublish(outboxPublishRuntime, "MARK"),
  "ALREADY_PUBLISHED",
);
const expiredLeaseRuntime = {
  outbox: {
    ...clone(causallyValidOutbox),
    publication_state: "PENDING",
  },
  brokerDeliveries: [],
  acknowledged: new Set(),
};
delete expiredLeaseRuntime.outbox.published_at;
assert.equal(
  executeOutboxPublish(
    expiredLeaseRuntime,
    "CLAIM_COMMIT",
    "outbox-publisher-a",
    "2026-07-29T10:00:02Z",
  ),
  "CLAIMED_NOT_PUBLISHED",
);
assert.equal(
  executeOutboxPublish(
    expiredLeaseRuntime,
    "CLAIM_COMMIT",
    "outbox-publisher-b",
    "2026-07-29T10:00:33Z",
  ),
  "CLAIMED_NOT_PUBLISHED",
);
assert.equal(
  expiredLeaseRuntime.outbox.lease_owner,
  "outbox-publisher-b",
  "expired Outbox lease was not reclaimed",
);
const ordinaryEnrollment = transactions.workflows.find(
  ({ name }) => name === "ordinary_device_enrollment",
);
assert(ordinaryEnrollment.must_not_write.includes("prior_device_revocation"));
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
assert.equal(migration.outbox_business_event_deletion, "FORBIDDEN_IN_PHASE_1");
assertDeepEqual(recoveryPolicy.verification_gates, [
  "constraints",
  "aggregate_versions",
  "outbox_continuity",
  "audit_linkage",
  "fixture_checksums",
]);
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
assert.equal(failureInjection.coverage_requirements.every_ack_boundary, true);
assert.equal(
  failureInjection.coverage_requirements.every_transaction_manifest_workflow,
  true,
);
assert.equal(
  failureInjection.coverage_requirements.every_postgresql_transaction_segment,
  true,
);
assert.equal(
  failureInjection.coverage_requirements
    .zero_persistence_workflows_prove_no_write,
  true,
);

assert.equal(stateMachines.machines.challenge.ttl_seconds, 120);
assert.equal(stateMachines.machines.refresh.family_deadline_seconds, 2_592_000);
assert.equal(stateMachines.machines.refresh.individual_ttl_seconds, 604_800);
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

assert.equal(phase0Manifest.schema_version, acceptedPhase0ManifestVersion);
assert.equal(phase0Manifest.base_commit, acceptedPhase0BaseCommit);
assert.equal(phase0Manifest.hash_algorithm, "SHA-256");
assertDeepEqual(phase0Manifest.permitted_mutable_file, {
  path: "package.json",
  base_sha256: acceptedPackageBaseSha256,
  only_allowed_change:
    "add test:platform and invoke it from test without changing dependencies or existing commands",
});
for (const [relativePath, expectedHash] of Object.entries(
  phase0Manifest.immutable_files,
)) {
  const content = await readFile(path.join(contractsDirectory, relativePath));
  const acceptedBaseContent = execFileSync(
    "git",
    [
      "-C",
      repositoryDirectory,
      "show",
      `${acceptedPhase0BaseCommit}:contracts/${relativePath}`,
    ],
  );
  assert.equal(
    sha256(acceptedBaseContent),
    expectedHash,
    `Phase 0 manifest hash does not match accepted base: ${relativePath}`,
  );
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
    acceptedPhase0BaseCommit,
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

const packageJson = await readJson(
  path.join(contractsDirectory, "package.json"),
);
assertDeepEqual(packageJson, {
  name: "@fit-trade/contracts",
  version: "0.1.0",
  private: true,
  type: "module",
  scripts: {
    test: "npm run test:contracts && npm run test:platform && npm run test:history-secrets",
    "test:contracts": "node scripts/verify.mjs",
    "test:platform": "node scripts/verify-platform.mjs",
    "test:history-secrets":
      "git -C .. log -p --all -- . | node scripts/scan-stdin-secrets.mjs",
  },
  devDependencies: {
    "@apidevtools/swagger-parser": "12.1.0",
    ajv: "8.20.0",
    "ajv-formats": "3.0.1",
    protobufjs: "8.7.1",
    yaml: "2.9.0",
  },
});

const changedPaths = execFileSync(
  "git",
  [
    "-C",
    repositoryDirectory,
    "diff",
    "--name-only",
    acceptedPhase0BaseCommit,
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
  assert(
    allowedPath(changedPath),
    `P1-001 changed forbidden path ${changedPath}`,
  );
}
const untrackedContractPaths = execFileSync(
  "git",
  [
    "-C",
    repositoryDirectory,
    "ls-files",
    "--others",
    "--exclude-standard",
    "contracts",
  ],
  { encoding: "utf8" },
)
  .trim()
  .split("\n")
  .filter(Boolean);
for (const untrackedPath of untrackedContractPaths) {
  assert(
    allowedPath(untrackedPath),
    `P1-001 has an untracked forbidden path ${untrackedPath}`,
  );
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
    `Executed transaction cuts: ${executedTransactionCutCount}`,
    `Executed process-kill cuts: ${executedProcessKillCutCount}`,
    `Executed response-loss reconciliations: ${executedResponseLossCount}`,
    `Argon2 vector: ${argonVectorExecution}`,
    `Semantic scenarios: ${semanticScenarioCount}`,
    `Executable security scenario groups: ${executableSecurityScenarioCount}`,
    `Persistence linkage chains: ${linkageChains.chains.length}`,
    "Phase 1 platform contract verification passed.",
  ].join("\n"),
);
