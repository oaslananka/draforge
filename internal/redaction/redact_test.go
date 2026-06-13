// Package redaction unit tests.
// SPDX-License-Identifier: Apache-2.0
package redaction

import (
	"reflect"
	"testing"
)

func TestRedactString(t *testing.T) {
	dummyToken := "dop_v1_" + "1111111122222222333333334444444455555555666666667777777788888888"
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "normal text",
			input:    "This is a normal log line.",
			expected: "This is a normal log line.",
		},
		{
			name:     "digitalocean token",
			input:    "my token is " + dummyToken + " in logs.",
			expected: "my token is [REDACTED_DIGITALOCEAN_TOKEN] in logs.",
		},
		{
			name:     "PEM private key",
			input:    "Key data:\n-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEAn9...\n-----END RSA PRIVATE KEY-----\nEnd of key.",
			expected: "Key data:\n[REDACTED_PRIVATE_KEY]\nEnd of key.",
		},
		{
			name:     "kubeconfig credential data",
			input:    "client-certificate-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCg==",
			expected: "client-certificate-data: [REDACTED_BASE64_DATA]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := RedactString(tt.input)
			if actual != tt.expected {
				t.Errorf("RedactString() = %q, want %q", actual, tt.expected)
			}
		})
	}
}

func TestRedactMap(t *testing.T) {
	dummyToken := "dop_v1_" + "1111111122222222333333334444444455555555666666667777777788888888"
	inputMap := map[string]interface{}{
		"username": "admin",
		"password": "super-secret-password-123",
		"token":    dummyToken,
		"metadata": map[string]interface{}{
			"apiKey": "12345-abcde",
			"public": "visible-info",
		},
	}

	expectedMap := map[string]interface{}{
		"username": "admin",
		"password": "[REDACTED]",
		"token":    "[REDACTED]",
		"metadata": map[string]interface{}{
			"apiKey": "[REDACTED]",
			"public": "visible-info",
		},
	}

	actualMap := RedactMap(inputMap)
	if !reflect.DeepEqual(actualMap, expectedMap) {
		t.Errorf("RedactMap() = %v, want %v", actualMap, expectedMap)
	}
}
