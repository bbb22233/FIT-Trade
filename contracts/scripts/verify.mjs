import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import YAML from "yaml";

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
    "account.get_snapshot",
    "market.get_context",
    "operation.get_status",
    "order.cancel_entry",
    "position.close",
    "position.reduce",
    "protection.tighten_stop",
    "trade.propose_intent",
    "trade.request_confirmation",
    "trade.submit_feedback",
  ];
  assert.deepEqual(
    inventory.tools.map(({ name }) => name).sort(),
    expectedNames,
  );

  const expectedRisk = {
    "account.get_snapshot": "READ_ONLY",
    "market.get_context": "READ_ONLY",
    "operation.get_status": "READ_ONLY",
    "order.cancel_entry": "REDUCE_RISK",
    "position.close": "REDUCE_RISK",
    "position.reduce": "REDUCE_RISK",
    "protection.tighten_stop": "REDUCE_RISK",
    "trade.propose_intent": "PROPOSE_INCREASE",
    "trade.request_confirmation": "PROPOSE_INCREASE",
    "trade.submit_feedback": "LEARNING_ONLY",
  };
  const forbiddenToolPattern =
    /(raw|sign|private.?key|wallet|withdraw|transfer|sql|shell|exec|spawn)/i;
  const serverFields = new Set(["user_id", "account_id", "session_id"]);

  for (const tool of inventory.tools) {
    assert.equal(tool.risk_effect, expectedRisk[tool.name]);
    assert.ok(!forbiddenToolPattern.test(tool.name), `forbidden MCP name ${tool.name}`);
    const validateInput = ajv.compile(tool.input_schema);
    assert.equal(typeof validateInput, "function");
    walkObject(tool.input_schema, (key) => {
      assert.ok(
        !serverFields.has(key),
        `${tool.name}: model input exposes server field ${key}`,
      );
    });
  }
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

  verifyStateMachine(await readJson("state-machines/automation.json"));
}

async function verifyOpenApiAndProto() {
  const api = YAML.parse(await readText("openapi/openapi.yaml"));
  assert.equal(api.openapi, "3.1.0");
  assert.ok(Object.keys(api.paths).length >= 6);
  assert.deepEqual(api["x-websocket"].client_messages, []);

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
  assert.match(proto, /^syntax = "proto3";/);
  assert.match(proto, /\bpackage fit\.v1;/);
  for (const message of [
    "TradeIntent",
    "ConfirmationTicket",
    "Operation",
    "PositionSnapshot",
  ]) {
    assert.match(proto, new RegExp(`\\bmessage ${message}\\b`));
  }
  assert.match(proto, /\bservice TradingCore\b/);
  assert.doesNotMatch(proto, /\b(?:float|double)\b/);
}

async function verifyErrorAndFakeContracts() {
  const taxonomy = await readText("spec/error-taxonomy.md");
  const codes = [...taxonomy.matchAll(/\| `([A-Z][A-Z0-9_]+)` \|/g)].map(
    ([, code]) => code,
  );
  assert.ok(codes.length >= 12);
  assert.equal(new Set(codes).size, codes.length);
  assert.ok(codes.includes("UNKNOWN_REQUIRES_RECONCILIATION"));
  assert.match(taxonomy, /blind retry is forbidden/i);

  const fake = await readJson("fake-hyperliquid-scenarios-v1.json");
  assert.equal(fake.schema_version, "fit.fake-hyperliquid.v1");
  assert.equal(fake.network_access, false);
  assert.equal(fake.synthetic_only, true);
  assert.equal(
    new Set(fake.scenarios.map(({ id }) => id)).size,
    fake.scenarios.length,
  );
  for (const id of [
    "dispatch_timeout_after_send",
    "duplicate_event",
    "partial_fill_protection_failed",
    "stale_market_snapshot",
  ]) {
    assert.ok(fake.scenarios.some((scenario) => scenario.id === id));
  }
}

async function verifySecretBaseline() {
  const roots = [
    "README.md",
    "confirmation-fields.json",
    "mcp-tools-v1.json",
    "jsonschema",
    "state-machines",
    "fixtures",
    "proto",
    "openapi",
    "spec",
    "security",
  ];
  const files = [];

  async function collect(relativePath) {
    const absolute = path.join(root, relativePath);
    const entries = await readdir(absolute, { withFileTypes: true }).catch(
      () => null,
    );
    if (entries === null) {
      files.push(relativePath);
      return;
    }
    for (const entry of entries) {
      const child = path.join(relativePath, entry.name);
      if (entry.isDirectory()) await collect(child);
      else files.push(child);
    }
  }

  for (const relativePath of roots) await collect(relativePath);
  const patterns = [
    /-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/,
    /\b(?:sk|pk)_(?:live|test)_[A-Za-z0-9]{16,}/,
    /\b(?:API_KEY|SECRET_KEY|PRIVATE_KEY)\s*[:=]\s*["'][^"' \n]{8,}/,
    /\bBearer\s+[A-Za-z0-9._-]{20,}/,
  ];
  for (const relativePath of files.sort()) {
    const content = await readText(relativePath);
    for (const pattern of patterns) {
      assert.doesNotMatch(content, pattern, `${relativePath}: secret-like value`);
    }
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
