import { resolve } from "node:path";
import { generate, root } from "./protocol-lib.mjs";

generate(
  resolve(root, "protocol/generated/go/types.gen.go"),
  resolve(root, "protocol/generated/typescript/schema.d.ts"),
);
