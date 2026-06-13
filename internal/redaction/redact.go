// Package redaction provides utilities to scrub secrets from diagnostics, logs, and manifests.
// SPDX-License-Identifier: Apache-2.0
package redaction

import (
	"regexp"
	"strings"
)

var (
	// Secret key regexes
	keyRegex = regexp.MustCompile(`(?i)(token|password|secret|key|auth|cert|credential|private|pass|pwd)`)

	// Value regexes for common secret patterns
	jwtRegex        = regexp.MustCompile(`eyJhbGciOi[A-Za-z0-9-_=]+\.[A-Za-z0-9-_=]+\.?[A-Za-z0-9-_.+/=]*`)
	doTokenRegex    = regexp.MustCompile(`dop_v1_[a-f0-9]{64}`)
	pemPrivateRegex = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	pemCertRegex    = regexp.MustCompile(`(?s)-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----`)
	kubeconfigRegex = regexp.MustCompile(`(?i)(client-certificate-data|client-key-data|certificate-authority-data):\s*[A-Za-z0-9+/=]+`)
)

// RedactString sanitizes a raw string using regex matches.
func RedactString(input string) string {
	if input == "" {
		return ""
	}

	// Redact DigitalOcean Token
	output := doTokenRegex.ReplaceAllString(input, "[REDACTED_DIGITALOCEAN_TOKEN]")

	// Redact JWT tokens (e.g. service account tokens)
	output = jwtRegex.ReplaceAllString(output, "[REDACTED_JWT_TOKEN]")

	// Redact PEM private keys
	output = pemPrivateRegex.ReplaceAllString(output, "[REDACTED_PRIVATE_KEY]")

	// Redact PEM certificates
	output = pemCertRegex.ReplaceAllString(output, "[REDACTED_CERTIFICATE]")

	// Redact base64 kubeconfig certs/keys
	output = kubeconfigRegex.ReplaceAllString(output, "$1: [REDACTED_BASE64_DATA]")

	return output
}

// RedactMap recursively scrubs sensitive keys from key-value map structures.
func RedactMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}

	result := make(map[string]interface{})
	for k, v := range m {
		// Check if key is sensitive
		if keyRegex.MatchString(k) {
			result[k] = "[REDACTED]"
			continue
		}

		// Recurse if nested map
		if nestedMap, ok := v.(map[string]interface{}); ok {
			result[k] = RedactMap(nestedMap)
		} else if strVal, ok := v.(string); ok {
			result[k] = RedactString(strVal)
		} else {
			result[k] = v
		}
	}
	return result
}
