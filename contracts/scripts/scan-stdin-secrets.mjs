import { readFileSync } from "node:fs";

import { matchingSecretPatternNames } from "./secret-patterns.mjs";

const content = readFileSync(0, "utf8");
const matches = matchingSecretPatternNames(content);

if (matches.length > 0) {
  throw new Error(`secret-like values detected: ${matches.join(", ")}`);
}

console.log("FIT-Trade Git history secret scan: PASS");
