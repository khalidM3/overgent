import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

test("fixture conforms to the event envelope schema", async () => {
  const schema = JSON.parse(await readFile(new URL("../../../protocol/schemas/event-envelope.schema.json", import.meta.url)));
  const manifestSchema = JSON.parse(await readFile(new URL("../../../protocol/schemas/change-manifest.schema.json", import.meta.url)));
  const verificationSchema = JSON.parse(await readFile(new URL("../../../protocol/schemas/verification.schema.json", import.meta.url)));
  const fixture = JSON.parse(await readFile(new URL("../../../protocol/fixtures/workspace-manifest-completed.json", import.meta.url)));
  const ajv = new Ajv2020({ allErrors: true, strictTypes: false });
  addFormats(ajv);
  ajv.addSchema(manifestSchema);
  ajv.addSchema(verificationSchema);
  const validate = ajv.compile(schema);
  assert.equal(validate(fixture), true, JSON.stringify(validate.errors));
  const invalid = structuredClone(fixture);
  invalid.payload.pathCount = 1;
  assert.equal(validate(invalid), false, "type-specific payload accepted an undeclared field");
  const emptyStart = structuredClone(fixture);
  emptyStart.type = "workspace.manifest_started";
  emptyStart.payload = {
    manifestId: "mft_empty",
    revision: 8,
    workstreamId: "wrk_fixture",
    baselineRef: "0".repeat(40),
    headRef: "1".repeat(40),
    chunkCount: 0,
  };
  assert.equal(validate(emptyStart), true, JSON.stringify(validate.errors));
});
