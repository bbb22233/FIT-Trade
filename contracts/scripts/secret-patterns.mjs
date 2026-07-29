export const secretPatterns = [
  {
    name: "pem-private-key",
    pattern: /-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/,
  },
  {
    name: "provider-key-underscore",
    pattern: /\b(?:sk|pk)_(?:live|test)_[A-Za-z0-9]{16,}/,
  },
  {
    name: "provider-key-hyphen",
    pattern: /\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}/,
  },
  {
    name: "github-token",
    pattern: /\b(?:gh[pousr]_[A-Za-z0-9]{36,255}|github_pat_[A-Za-z0-9_]{40,255})\b/,
  },
  {
    name: "aws-access-key",
    pattern: /\b(?:AKIA|ASIA)[0-9A-Z]{16}\b/,
  },
  {
    name: "jwt",
    pattern: /\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b/,
  },
  {
    name: "credential-url",
    pattern: /\b[a-z][a-z0-9+.-]*:\/\/[^/\s:@]+:[^/\s@]{8,}@[^\s/]+/i,
  },
  {
    name: "quoted-secret-assignment",
    pattern:
      /\b(?:API_KEY|SECRET_KEY|PRIVATE_KEY|ACCESS_TOKEN|AUTH_TOKEN|PASSWORD)\s*[:=]\s*["'][^"' \n]{8,}/i,
  },
  {
    name: "unquoted-secret-assignment",
    pattern:
      /\b(?:API_KEY|SECRET_KEY|PRIVATE_KEY|ACCESS_TOKEN|AUTH_TOKEN|PASSWORD)\s*[:=]\s*[A-Za-z0-9_+./-]{12,}/i,
  },
  {
    name: "bearer-token",
    pattern: /\bBearer\s+[A-Za-z0-9._-]{20,}/,
  },
  {
    name: "slack-token",
    pattern: /\bxox[baprs]-[A-Za-z0-9-]{20,}\b/,
  },
  {
    name: "seed-phrase-assignment",
    pattern:
      /(?:mnemonic|seed[_-]?phrase)\s*[:=]\s*["'](?:[a-z]+\s+){11,23}[a-z]+["']/i,
  },
];

export function matchingSecretPatternNames(content) {
  return secretPatterns
    .filter(({ pattern }) => pattern.test(content))
    .map(({ name }) => name);
}
