import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

test("fixture conforms to the event envelope schema", async () => {
  const schema = JSON.parse(await readFile(new URL("../../../protocol/schemas/event-envelope.schema.json", import.meta.url)));
  const fixture = JSON.parse(await readFile(new URL("../../../protocol/fixtures/workspace-manifest-completed.json", import.meta.url)));
  const ajv = new Ajv2020({ allErrors: true });
  addFormats(ajv);
  const validate = ajv.compile(schema);
  assert.equal(validate(fixture), true, JSON.stringify(validate.errors));
});
