import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import SwaggerParser from "@apidevtools/swagger-parser";
import protobuf from "protobufjs";
import YAML from "yaml";

import { matchingSecretPatternNames } from "./secret-patterns.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

async function readText(relativePath) {
  return readFile(path.join(root, relativePath), "utf8");
}

async function readJson(relativePath) {
  return JSON.parse(await readText(relativePath));
}

async function jsonFiles(relativeDirectory) {
  const names = await readdir(path.join(root, relativeDirectory));
  return names
    .filter((name) => name.endsWith(".json"))
    .sort()
    .map((name) => path.join(relativeDirectory, name));
}

function canonicalize(value) {
  if (value === null || typeof value === "boolean" || typeof value === "number") {
    assert.ok(
      typeof value !== "number" || Number.isFinite(value),
      "JCS forbids non-finite numbers",
    );
    return JSON.stringify(value);
  }
  if (typeof value === "string") {
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) {
    return `[${value.map(canonicalize).join(",")}]`;
  }
  assert.equal(typeof value, "object", "unsupported JCS value");
  const keys = Object.keys(value).sort();
  return `{${keys
    .map((key) => `${JSON.stringify(key)}:${canonicalize(value[key])}`)
    .join(",")}}`;
}

function sha256Utf8(value) {
  return createHash("sha256").update(value, "utf8").digest("hex");
}

function valueAtPath(value, dottedPath) {
  let current = value;
  for (const segment of dottedPath.split(".")) {
    assert.ok(
      current !== null &&
        typeof current === "object" &&
        Object.hasOwn(current, segment),
      `confirmation field is missing: ${dottedPath}`,
    );
    current = current[segment];
  }
  return current;
}

function confirmationBinding(ticket, fields) {
  return Object.fromEntries(
    fields.map((field) => [field, valueAtPath(ticket, field)]),
  );
}

function mutatedValue(value) {
  if (typeof value === "string") return `${value}x`;
  if (typeof value === "number") return value + 1;
  if (typeof value === "boolean") return !value;
  if (Array.isArray(value)) return [...value, { mutation: true }];
  if (value === null) return "mutation";
  return { ...value, mutation: true };
}

function setAtPath(value, dottedPath, replacement) {
  const segments = dottedPath.split(".");
  let current = value;
  for (const segment of segments.slice(0, -1)) current = current[segment];
  current[segments.at(-1)] = replacement;
}

function decimalParts(value) {
  const negative = value.startsWith("-");
  const unsigned = negative ? value.slice(1) : value;
  const [whole, fraction = ""] = unsigned.split(".");
  return {
    coefficient: BigInt(`${whole}${fraction}`) * (negative ? -1n : 1n),
    scale: fraction.length,
  };
}

function equalAbsoluteDecimal(left, right) {
  const a = decimalParts(left);
  const b = decimalParts(right);
  const scale = Math.max(a.scale, b.scale);
  const scaledA =
    (a.coefficient < 0n ? -a.coefficient : a.coefficient) *
    10n ** BigInt(scale - a.scale);
  const scaledB = b.coefficient * 10n ** BigInt(scale - b.scale);
  return scaledA === scaledB;
}

function validProtectionAggregate(position, protection) {
  return (
    position.protection_state === "PROTECTED" &&
    protection.state === "PROTECTED" &&
    position.protection_status_id === protection.protection_status_id &&
    position.position_id === protection.position_id &&
    equalAbsoluteDecimal(
      position.signed_quantity,
      protection.absolute_live_position_quantity,
    ) &&
    protection.active_stop_order_ids.length >= 1 &&
    /^[a-f0-9]{64}$/.test(protection.coverage_evidence_hash)
  );
}

function walkObject(value, visit, currentPath = []) {
  if (value === null || typeof value !== "object") return;
  if (Array.isArray(value)) {
    value.forEach((item, index) => walkObject(item, visit, [...currentPath, index]));
    return;
  }
  for (const [key, child] of Object.entries(value)) {
    visit(key, child, currentPath);
    walkObject(child, visit, [...currentPath, key]);
  }
}

function verifyStateMachine(machine) {
  assert.equal(typeof machine.name, "string");
  assert.ok(machine.states.includes(machine.initial));
  assert.equal(new Set(machine.states).size, machine.states.length);
  assert.ok(machine.terminal.every((state) => machine.states.includes(state)));

  const transitions = new Set();
  for (const transition of machine.transitions) {
    assert.equal(transition.length, 2);
    const [from, to] = transition;
    assert.ok(machine.states.includes(from), `${machine.name}: unknown from state`);
    assert.ok(machine.states.includes(to), `${machine.name}: unknown to state`);
    const key = `${from}->${to}`;
    assert.ok(!transitions.has(key), `${machine.name}: duplicate transition ${key}`);
    transitions.add(key);
  }

  for (const terminal of machine.terminal) {
    assert.ok(
      !machine.transitions.some(([from]) => from === terminal),
      `${machine.name}: terminal ${terminal} has an outgoing edge`,
    );
  }

  for (const from of machine.states) {
    for (const to of machine.states) {
      const allowed = transitions.has(`${from}->${to}`);
      if (!allowed) {
        assert.equal(
          transitions.has(`${from}->${to}`),
          false,
          `${machine.name}: illegal transition accepted`,
        );
      }
    }
  }

  for (const guardedTransition of Object.keys(machine.guards ?? {})) {
    assert.ok(
      transitions.has(guardedTransition),
      `${machine.name}: guard without transition ${guardedTransition}`,
    );
  }

  return transitions;
}

async function verifyDomainFixtures() {
  const schema = await readJson("jsonschema/fit-trade-v1.schema.json");
  const ajv = new Ajv2020({ allErrors: true, strict: true });
  addFormats(ajv);
  ajv.addSchema(schema);

  const validators = new Map();
  for (const name of Object.keys(schema.$defs)) {
    validators.set(
      name,
      ajv.compile({ $ref: `${schema.$id}#/$defs/${name}` }),
    );
  }

  for (const fixturePath of await jsonFiles("fixtures/valid")) {
    const fixture = await readJson(fixturePath);
    const validate = validators.get(fixture.schema);
    assert.ok(validate, `${fixturePath}: unknown schema ${fixture.schema}`);
    assert.ok(
      validate(fixture.value),
      `${fixturePath}: expected valid: ${ajv.errorsText(validate.errors)}`,
    );
  }

  for (const fixturePath of await jsonFiles("fixtures/invalid")) {
    const fixture = await readJson(fixturePath);
    const validate = validators.get(fixture.schema);
    assert.ok(validate, `${fixturePath}: unknown schema ${fixture.schema}`);
    assert.equal(
      validate(fixture.value),
      false,
      `${fixturePath}: invalid fixture was accepted (${fixture.reason})`,
    );
  }

  const coverageDocument = await readJson("fixtures/matrix/domain-values.json");
  const { _matrix_version: matrixVersion, ...coverage } = coverageDocument;
  assert.equal(matrixVersion, "fit.domain-coverage.v1");
  const topLevelNames = schema.oneOf
    .map(({ $ref }) => $ref.split("/").at(-1))
    .sort();
  assert.deepEqual(Object.keys(coverage).sort(), topLevelNames);

  const boundaryMutations = {
    TradeIntent: ["leverage", 101],
    ConfirmationTicket: ["maximum_loss_fraction", "1.1"],
    Operation: ["state_version", -1],
    ExecutionAttempt: ["attempt_number", 0],
    Order: ["client_order_id", "short"],
    Fill: ["quantity", "0"],
    PositionSnapshot: ["protection_status_id", "not-a-uuid"],
    RiskPolicy: ["maximum_trade_risk_fraction", "1.1"],
    AutomationGrant: ["allowed_symbols", []],
    ModelProposal: ["model_version", "x".repeat(65)],
    ModelReview: ["reason_codes", []],
    RiskDecision: ["post_trade_total_risk_fraction", "1.1"],
    ExecutionCommand: ["quantity", "0"],
    ExecutionResult: ["status", "SUCCESS"],
    ProtectionStatus: ["data_status", "UNKNOWN"],
    ReconciliationStatus: ["evidence_sources", []],
    AgentFeedback: ["rating", 3],
    AutomationAuthorization: ["allowed_symbols", []],
    AuditEvent: ["event_type", ""],
  };
  const maliciousPaths = {
    TradeIntent: "intent_id",
    ConfirmationTicket: "confirmation_id",
    Operation: "operation_id",
    ExecutionAttempt: "attempt_id",
    Order: "order_id",
    Fill: "fill_id",
    PositionSnapshot: "position_id",
    RiskPolicy: "risk_policy_version",
    AutomationGrant: "grant_id",
    ModelProposal: "proposal_id",
    ModelReview: "review_id",
    RiskDecision: "decision_id",
    ExecutionCommand: "command_id",
    ExecutionResult: "result_id",
    ProtectionStatus: "position_id",
    ReconciliationStatus: "reconciliation_id",
    AgentFeedback: "feedback_id",
    AutomationAuthorization: "authorization_id",
    AuditEvent: "event_id",
  };

  for (const name of topLevelNames) {
    const validate = validators.get(name);
    const validateRoot = ajv.getSchema(schema.$id);
    const valid = coverage[name];
    assert.ok(
      validate(valid),
      `${name}: coverage positive invalid: ${ajv.errorsText(validate.errors)}`,
    );
    assert.ok(
      validateRoot(valid),
      `${name}: root oneOf rejected valid object: ${ajv.errorsText(validateRoot.errors)}`,
    );

    const firstRequired = schema.$defs[name].required[0];
    const missing = structuredClone(valid);
    delete missing[firstRequired];
    assert.equal(validate(missing), false, `${name}: missing-field case accepted`);

    const unknown = { ...structuredClone(valid), unexpected_field: "rejected" };
    assert.equal(validate(unknown), false, `${name}: unknown field accepted`);

    const boundary = structuredClone(valid);
    setAtPath(boundary, boundaryMutations[name][0], boundaryMutations[name][1]);
    assert.equal(validate(boundary), false, `${name}: boundary case accepted`);

    const malicious = structuredClone(valid);
    setAtPath(malicious, maliciousPaths[name], "../../etc/passwd");
    assert.equal(validate(malicious), false, `${name}: malicious case accepted`);
  }

  const confirmationValidator = validators.get("ConfirmationTicket");
  const withoutTakeProfit = structuredClone(coverage.ConfirmationTicket);
  delete withoutTakeProfit.intent.take_profit_plan;
  assert.equal(confirmationValidator(withoutTakeProfit), false);
  const closeTicket = structuredClone(coverage.ConfirmationTicket);
  closeTicket.intent.position_effect = "CLOSE";
  delete closeTicket.intent.stop;
  assert.equal(confirmationValidator(closeTicket), false);

  const intentValidator = validators.get("TradeIntent");
  const limitIoc = structuredClone(coverage.TradeIntent);
  limitIoc.order_type = "LIMIT";
  limitIoc.time_in_force = "IOC";
  assert.equal(intentValidator(limitIoc), false);
  const proposalValidator = validators.get("ModelProposal");
  const tradeProposal = {
    ...structuredClone(coverage.ModelProposal),
    decision: "PROPOSE_TRADE",
    intent: structuredClone(coverage.TradeIntent),
  };
  assert.ok(proposalValidator(tradeProposal));

  const commandValidator = validators.get("ExecutionCommand");
  const limitIocCommand = structuredClone(coverage.ExecutionCommand);
  limitIocCommand.order_type = "LIMIT";
  limitIocCommand.time_in_force = "IOC";
  assert.equal(commandValidator(limitIocCommand), false);

  const invariants = await readJson("semantic-invariants-v1.json");
  assert.equal(invariants.schema_version, "fit.semantic-invariants.v1");
  assert.ok(
    invariants.invariants.some(({ id }) => id === "PROTECTION_FULL_COVERAGE"),
  );
  assert.ok(
    validProtectionAggregate(
      coverage.PositionSnapshot,
      coverage.ProtectionStatus,
    ),
  );
  const legacyPositionContradiction = {
    ...structuredClone(coverage.PositionSnapshot),
    protected_quantity: "0",
  };
  assert.equal(
    validators.get("PositionSnapshot")(legacyPositionContradiction),
    false,
  );
  const legacyProtectionContradiction = {
    ...structuredClone(coverage.ProtectionStatus),
    protected_quantity: "0",
  };
  assert.equal(
    validators.get("ProtectionStatus")(legacyProtectionContradiction),
    false,
  );
  const structurallyUnprotected = structuredClone(coverage.ProtectionStatus);
  structurallyUnprotected.absolute_live_position_quantity = "0";
  structurallyUnprotected.active_stop_order_ids = [];
  delete structurallyUnprotected.coverage_evidence_hash;
  assert.equal(
    validators.get("ProtectionStatus")(structurallyUnprotected),
    false,
  );
  const underCovered = structuredClone(coverage.ProtectionStatus);
  underCovered.absolute_live_position_quantity = "0.01";
  assert.equal(
    validProtectionAggregate(coverage.PositionSnapshot, underCovered),
    false,
  );
  const missingStop = structuredClone(coverage.ProtectionStatus);
  missingStop.active_stop_order_ids = [];
  assert.equal(
    validProtectionAggregate(coverage.PositionSnapshot, missingStop),
    false,
  );

  return { schema, validators };
}

async function verifyConfirmationHash(validators) {
  const definition = await readJson("confirmation-fields.json");
  assert.equal(definition.schema_version, "fit.confirmation-hash.v1");
  assert.equal(definition.algorithm, "sha256");
  assert.equal(definition.canonicalization, "RFC8785-JCS");
  assert.equal(new Set(definition.fields).size, definition.fields.length);

  const fixture = await readJson("fixtures/valid/confirmation-ticket-open.json");
  const validate = validators.get("ConfirmationTicket");
  assert.ok(validate(fixture.value), "golden confirmation must validate");

  const binding = confirmationBinding(fixture.value, definition.fields);
  const canonical = canonicalize(binding);
  const digest = sha256Utf8(canonical);
  const golden = (await readText("fixtures/golden/confirmation-open.sha256")).trim();
  assert.equal(
    digest,
    golden,
    `confirmation golden mismatch; computed ${digest}`,
  );
  assert.equal(fixture.value.confirmation_hash, golden);

  const reversedBinding = Object.fromEntries(Object.entries(binding).reverse());
  assert.equal(sha256Utf8(canonicalize(reversedBinding)), digest);

  for (const field of definition.fields) {
    const mutation = structuredClone(binding);
    mutation[field] = mutatedValue(mutation[field]);
    assert.notEqual(
      sha256Utf8(canonicalize(mutation)),
      digest,
      `confirmation digest ignored ${field}`,
    );
  }
}

async function verifyMcpInventory() {
  const schema = await readJson("jsonschema/mcp-tools-v1.schema.json");
  const inventory = await readJson("mcp-tools-v1.json");
  const ajv = new Ajv2020({ allErrors: true, strict: true });
  addFormats(ajv);
  const validateInventory = ajv.compile(schema);
  assert.ok(
    validateInventory(inventory),
    `MCP inventory invalid: ${ajv.errorsText(validateInventory.errors)}`,
  );

  const expectedNames = [
    "cancel_entry_order",
    "create_trade_intent",
    "get_account_state",
    "get_market_snapshot",
    "get_open_orders",
    "get_positions",
    "get_risk_limits",
    "request_reduce_position",
    "submit_trade_feedback",
    "tighten_stop",
  ];
  assert.deepEqual(
    inventory.tools.map(({ name }) => name).sort(),
    expectedNames,
  );

  const expectedRisk = {
    cancel_entry_order: "REDUCE_RISK",
    create_trade_intent: "PROPOSE_INCREASE",
    get_account_state: "READ_ONLY",
    get_market_snapshot: "READ_ONLY",
    get_open_orders: "READ_ONLY",
    get_positions: "READ_ONLY",
    get_risk_limits: "READ_ONLY",
    request_reduce_position: "REDUCE_RISK",
    submit_trade_feedback: "LEARNING_ONLY",
    tighten_stop: "REDUCE_RISK",
  };
  const forbiddenToolPattern =
    /(raw|sign|private.?key|wallet|withdraw|transfer|sql|shell|exec|spawn)/i;
  const serverFields = new Set(["user_id", "account_id", "session_id"]);

  for (const tool of inventory.tools) {
    assert.equal(tool.risk_effect, expectedRisk[tool.name]);
    assert.ok(!forbiddenToolPattern.test(tool.name), `forbidden MCP name ${tool.name}`);
    const validateInput = ajv.compile(tool.input_schema);
    assert.equal(typeof validateInput, "function");
    for (const field of serverFields) {
      assert.equal(
        validateInput({ [field]: "22222222-2222-4222-8222-222222222222" }),
        false,
        `${tool.name}: model supplied ${field} was accepted`,
      );
    }
    walkObject(tool.input_schema, (key) => {
      assert.ok(
        !serverFields.has(key),
        `${tool.name}: model input exposes server field ${key}`,
      );
    });
  }

  const createIntent = inventory.tools.find(
    ({ name }) => name === "create_trade_intent",
  );
  const validateCreateIntent = ajv.compile(createIntent.input_schema);
  const validCreateInput = {
    symbol: "BTC-PERP",
    side: "BUY",
    position_effect: "OPEN",
    order_type: "MARKET",
    time_in_force: "IOC",
    quantity: "0.025",
    margin_mode: "ISOLATED",
    leverage: 5,
    entry_price_or_bound: "118452",
    worst_acceptable_price: "118689",
    stop_trigger: "115200",
  };
  assert.ok(validateCreateIntent(validCreateInput));
  assert.equal(
    validateCreateIntent({ ...validCreateInput, user_id: "injected" }),
    false,
  );
  assert.equal(
    validateCreateIntent({
      ...validCreateInput,
      order_type: "LIMIT",
      time_in_force: "IOC",
    }),
    false,
  );

  const architecture = await readText("../docs/02_SYSTEM_ARCHITECTURE.md");
  const approvedBlock = architecture
    .split("允许的工具：")[1]
    .split("禁止的工具：")[0];
  const approvedNames = [...approvedBlock.matchAll(/`([^`]+)`/g)]
    .map(([, name]) => name)
    .sort();
  assert.deepEqual(approvedNames, expectedNames);
}

async function verifyStateMachines() {
  const operation = verifyStateMachine(
    await readJson("state-machines/operation.json"),
  );
  assert.ok(!operation.has("DISPATCHED->DISPATCH_PENDING"));
  assert.ok(!operation.has("UNKNOWN_REQUIRES_RECONCILIATION->DISPATCHED"));

  const protection = await readJson("state-machines/protection.json");
  const protectionTransitions = verifyStateMachine(protection);
  assert.equal(
    protection.invariant,
    "protected_quantity >= absolute_live_position_quantity",
  );
  assert.ok(
    !protectionTransitions.has("PROTECTION_FAILED->PROTECTED"),
    "failed protection cannot silently recover",
  );
  assert.ok(protectionTransitions.has("PROTECTED->PARTIALLY_FILLED"));
  assert.ok(protectionTransitions.has("PROTECTION_PENDING->PARTIALLY_FILLED"));

  verifyStateMachine(await readJson("state-machines/automation.json"));
}

async function verifyOpenApiAndProto() {
  const openApiPath = path.join(root, "openapi/openapi.yaml");
  const api = YAML.parse(await readText("openapi/openapi.yaml"));
  const parsedApi = await SwaggerParser.validate(openApiPath);
  assert.equal(parsedApi.openapi, "3.1.0");
  assert.equal(api.openapi, "3.1.0");
  assert.ok(Object.keys(api.paths).length >= 6);
  assert.deepEqual(api["x-websocket"].client_messages, []);
  assert.deepEqual(
    api.paths["/v1/confirmations/{confirmation_id}/consume"].post[
      "x-server-injected-scope"
    ],
    ["user_id", "account_id", "device_id", "session_id"],
  );
  const consumeBody =
    api.paths["/v1/confirmations/{confirmation_id}/consume"].post.requestBody
      .content["application/json"].schema;
  assert.deepEqual(consumeBody.required, ["confirmation_hash"]);
  assert.ok(!Object.hasOwn(consumeBody.properties, "device_challenge_signature"));

  const operationIds = [];
  for (const pathItem of Object.values(api.paths)) {
    for (const operation of Object.values(pathItem)) {
      if (operation && typeof operation === "object" && operation.operationId) {
        operationIds.push(operation.operationId);
      }
    }
  }
  assert.equal(new Set(operationIds).size, operationIds.length);
  assert.ok(operationIds.includes("consumeTradeConfirmation"));
  assert.ok(!Object.keys(api.paths).some((route) => /sign|withdraw|transfer/i.test(route)));

  const proto = await readText("proto/fit/v1/trading.proto");
  const parsedProto = protobuf.parse(proto, { keepCase: true });
  assert.ok(parsedProto.root.lookupType("fit.v1.TradeIntent"));
  assert.ok(parsedProto.root.lookupService("fit.v1.TradingCore"));
  const domainSchema = await readJson("jsonschema/fit-trade-v1.schema.json");
  const matrix = await readJson("fixtures/matrix/domain-values.json");
  const roundTripAjv = new Ajv2020({ allErrors: true, strict: true });
  addFormats(roundTripAjv);
  roundTripAjv.addSchema(domainSchema);
  const topLevelDomainNames = domainSchema.oneOf.map(({ $ref }) =>
    $ref.split("/").at(-1),
  );
  for (const name of topLevelDomainNames) {
    const protoType = parsedProto.root.lookupType(`fit.v1.${name}`);
    const protoFields = Object.keys(protoType.fields).sort();
    const schemaFields = Object.keys(domainSchema.$defs[name].properties).sort();
    assert.deepEqual(
      protoFields,
      schemaFields,
      `${name}: Proto and JSON Schema fields diverge`,
    );

    const sample = matrix[name];
    const encoded = protoType.encode(protoType.fromObject(sample)).finish();
    const decoded = protoType.toObject(protoType.decode(encoded), {
      arrays: true,
      enums: String,
      longs: Number,
    });
    const validateRoundTrip = roundTripAjv.compile({
      $ref: `${domainSchema.$id}#/$defs/${name}`,
    });
    assert.ok(
      validateRoundTrip(decoded),
      `${name}: Proto round trip violates JSON Schema: ${roundTripAjv.errorsText(
        validateRoundTrip.errors,
      )}`,
    );
  }
  for (const nestedName of ["StopMarket", "TakeProfitLeg", "SymbolLeverage"]) {
    const protoFields = Object.keys(
      parsedProto.root.lookupType(`fit.v1.${nestedName}`).fields,
    ).sort();
    const schemaFields = Object.keys(
      domainSchema.$defs[nestedName].properties,
    ).sort();
    assert.deepEqual(protoFields, schemaFields);
  }
  assert.equal(
    parsedProto.root.lookupType("fit.v1.RiskPolicy").fields
      .maximum_leverage_by_symbol.repeated,
    true,
  );
  assert.match(proto, /^syntax = "proto3";/);
  assert.match(proto, /\bpackage fit\.v1;/);
  for (const message of [
    "TradeIntent",
    "ConfirmationTicket",
    "Operation",
    "ExecutionAttempt",
    "Order",
    "Fill",
    "PositionSnapshot",
    "RiskPolicy",
    "AutomationGrant",
    "ModelProposal",
    "ModelReview",
    "AuditEvent",
    "AuthenticatedScope",
    "RiskDecision",
    "ExecutionCommand",
    "ExecutionResult",
    "ProtectionStatus",
    "ReconciliationStatus",
    "AgentFeedback",
    "AutomationAuthorization",
    "ErrorDetail",
  ]) {
    assert.match(proto, new RegExp(`\\bmessage ${message}\\b`));
  }
  assert.match(proto, /\bservice TradingCore\b/);
  assert.doesNotMatch(proto, /\b(?:float|double)\b/);
  const consumeMessage = proto.match(
    /message ConsumeConfirmationRequest \{([\s\S]*?)\n\}/,
  )?.[1];
  assert.ok(consumeMessage);
  assert.match(consumeMessage, /AuthenticatedScope scope/);
  assert.doesNotMatch(consumeMessage, /device_challenge_signature/);

  const taxonomy = await readJson("error-taxonomy-v1.json");
  const protoErrorCodes = Object.keys(
    parsedProto.root.lookupEnum("fit.v1.ErrorCode").values,
  ).filter((value) => value !== "ERROR_CODE_UNSPECIFIED");
  const protoRetryClasses = Object.keys(
    parsedProto.root.lookupEnum("fit.v1.RetryClass").values,
  ).filter((value) => value !== "RETRY_CLASS_UNSPECIFIED");
  assert.deepEqual(
    protoErrorCodes,
    taxonomy.errors.map(({ code }) => code),
  );
  assert.deepEqual(
    protoRetryClasses.sort(),
    [...new Set(taxonomy.errors.map(({ retry }) => retry))].sort(),
  );
  const apiPairs =
    api.components.schemas.Problem.allOf[0].oneOf.map(({ properties }) => ({
      code: properties.code.const,
      retry: properties.retry.const,
    }));
  assert.deepEqual(apiPairs, taxonomy.errors);
  const problemAjv = new Ajv2020({ allErrors: true, strict: true });
  addFormats(problemAjv);
  const validateProblem = problemAjv.compile(api.components.schemas.Problem);
  for (const { code, retry } of taxonomy.errors) {
    assert.ok(
      validateProblem({
        code,
        retry,
        message: "Redacted",
        correlation_id: "11111111-1111-4111-8111-111111111111",
      }),
      `${code}: OpenAPI Problem pair rejected`,
    );
  }
  assert.equal(
    validateProblem({
      code: "UNKNOWN_REQUIRES_RECONCILIATION",
      retry: "BOUNDED_BACKOFF_BEFORE_DISPATCH",
      message: "Redacted",
      correlation_id: "11111111-1111-4111-8111-111111111111",
    }),
    false,
  );
}

async function verifyErrorAndFakeContracts() {
  const taxonomy = await readText("spec/error-taxonomy.md");
  const markdownPairs = [
    ...taxonomy.matchAll(
      /^\| `([A-Z][A-Z0-9_]+)` \| [^|]+ \| `([A-Z][A-Z0-9_]+)` \|/gm,
    ),
  ].map(([, code, retry]) => ({ code, retry }));
  const codes = markdownPairs.map(({ code }) => code);
  assert.ok(codes.length >= 12);
  assert.equal(new Set(codes).size, codes.length);
  assert.ok(codes.includes("UNKNOWN_REQUIRES_RECONCILIATION"));
  assert.match(taxonomy, /blind retry is forbidden/i);
  const machineTaxonomy = await readJson("error-taxonomy-v1.json");
  assert.equal(machineTaxonomy.schema_version, "fit.errors.v1");
  assert.deepEqual(
    machineTaxonomy.errors,
    markdownPairs,
  );
  assert.equal(
    new Set(machineTaxonomy.errors.map(({ code }) => code)).size,
    machineTaxonomy.errors.length,
  );

  const fake = await readJson("fake-hyperliquid-scenarios-v1.json");
  assert.equal(fake.schema_version, "fit.fake-hyperliquid.v1");
  assert.equal(fake.network_access, false);
  assert.equal(fake.synthetic_only, true);
  assert.equal(
    new Set(fake.scenarios.map(({ id }) => id)).size,
    fake.scenarios.length,
  );
  const requiredScenarioIds = [
    "dispatch_timeout_after_send",
    "duplicate_event",
    "executor_crash_after_exchange_success",
    "executor_crash_before_signing",
    "executor_crash_after_signing_before_submit",
    "executor_crash_after_submit_before_record",
    "executor_crash_after_record_before_report",
    "result_published_ack_lost",
    "duplicate_client_order_id",
    "partial_fill_then_parent_cancel",
    "rapid_consecutive_partial_fills",
    "protection_response_lost",
    "replacement_stop_create_failed_old_stop_active",
    "partial_fill_protection_failed",
    "emergency_close_residual_position",
    "manual_native_position_change",
    "database_restart_during_operation",
    "websocket_disconnect_and_resubscribe",
    "client_disconnect_then_duplicate_submit",
    "stale_market_snapshot",
  ];
  for (const id of requiredScenarioIds) {
    assert.ok(fake.scenarios.some((scenario) => scenario.id === id));
  }

  const logging = await readJson("logging-policy-v1.json");
  assert.equal(logging.schema_version, "fit.logging.v1");
  assert.equal(logging.drop_unknown_fields, true);
  assert.equal(
    logging.allowed_fields.some((field) => logging.redacted_fields.includes(field)),
    false,
  );
  const redacted = {};
  for (const [key, value] of Object.entries(logging.test_vector.input)) {
    if (logging.allowed_fields.includes(key)) redacted[key] = value;
    else if (logging.redacted_fields.includes(key)) {
      redacted[key] = logging.replacement;
    }
  }
  assert.deepEqual(redacted, logging.test_vector.expected);
}

async function verifySecretBaseline() {
  const repositoryRoot = path.resolve(root, "..");
  const files = [];
  const skippedDirectories = new Set([".git", "node_modules"]);
  const skippedExtensions = new Set([
    ".gif",
    ".ico",
    ".jpg",
    ".jpeg",
    ".pdf",
    ".png",
    ".webp",
  ]);

  async function collect(absoluteDirectory) {
    const entries = await readdir(absoluteDirectory, { withFileTypes: true });
    for (const entry of entries) {
      if (entry.isDirectory() && skippedDirectories.has(entry.name)) continue;
      const child = path.join(absoluteDirectory, entry.name);
      if (entry.isDirectory()) {
        await collect(child);
      } else if (!skippedExtensions.has(path.extname(entry.name).toLowerCase())) {
        files.push(child);
      }
    }
  }

  await collect(repositoryRoot);
  for (const absolutePath of files.sort()) {
    const content = await readFile(absolutePath, "utf8");
    assert.deepEqual(
      matchingSecretPatternNames(content),
      [],
      `${path.relative(repositoryRoot, absolutePath)}: secret-like value`,
    );
  }

  const syntheticDetections = [
    ["ghp_", "A".repeat(40)].join(""),
    ["AKIA", "A".repeat(16)].join(""),
    ["eyJ", "A".repeat(12), ".", "B".repeat(12), ".", "C".repeat(12)].join(""),
    ["https://user:", "longpassword", "@example.invalid"].join(""),
    ["API_KEY", "=", "A".repeat(24)].join(""),
    ["sk-", "A".repeat(32)].join(""),
    ["xoxb-", "A".repeat(24)].join(""),
  ];
  for (const synthetic of syntheticDetections) {
    assert.ok(
      matchingSecretPatternNames(synthetic).length >= 1,
      "known secret format was not detected",
    );
  }
}

async function main() {
  const { validators } = await verifyDomainFixtures();
  await verifyConfirmationHash(validators);
  await verifyMcpInventory();
  await verifyStateMachines();
  await verifyOpenApiAndProto();
  await verifyErrorAndFakeContracts();
  await verifySecretBaseline();
  console.log("FIT-Trade Phase 0 contract verification: PASS");
}

await main();
