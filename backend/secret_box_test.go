package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func testSecretKey(fill byte) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = fill
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func TestSecretBoxRoundTripUsesFreshNonce(t *testing.T) {
	box, err := newSecretBox(testSecretKey(7))
	if err != nil {
		t.Fatal(err)
	}
	first, err := box.seal("plain-api-key-1234")
	if err != nil {
		t.Fatal(err)
	}
	second, err := box.seal("plain-api-key-1234")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(first, "plain-api-key") {
		t.Fatal("ciphertext must not contain the plain key")
	}
	if first == second {
		t.Fatal("two saves of the same key must use different nonces")
	}
	got, err := box.open(first)
	if err != nil || got != "plain-api-key-1234" {
		t.Fatalf("open() = %q, %v; want the original key", got, err)
	}
}

func TestSecretBoxDetectsTamperingAndWrongKey(t *testing.T) {
	box, _ := newSecretBox(testSecretKey(7))
	sealed, _ := box.seal("plain-api-key-1234")

	raw, _ := base64.StdEncoding.DecodeString(sealed)
	raw[len(raw)-1] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(raw)
	if _, err := box.open(tampered); err == nil {
		t.Fatal("a modified ciphertext must not open")
	}

	other, _ := newSecretBox(testSecretKey(9))
	if _, err := other.open(sealed); err == nil {
		t.Fatal("a ciphertext must not open with a different encryption key")
	}
	if _, err := box.open("!!not-base64"); err == nil {
		t.Fatal("garbage input must not open")
	}
}

func TestSecretBoxRejectsMissingOrWrongSizeKey(t *testing.T) {
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	for name, key := range map[string]string{"empty": "", "not base64": "%%%", "16 bytes": short} {
		if _, err := newSecretBox(key); err == nil {
			t.Errorf("%s: newSecretBox accepted an invalid key", name)
		}
	}
	t.Setenv("SETTINGS_ENCRYPTION_KEY", "")
	if newSecretBoxFromEnv() != nil {
		t.Fatal("an empty SETTINGS_ENCRYPTION_KEY must leave the web key unwritable")
	}
	t.Setenv("SETTINGS_ENCRYPTION_KEY", testSecretKey(3))
	if newSecretBoxFromEnv() == nil {
		t.Fatal("a valid SETTINGS_ENCRYPTION_KEY must enable the web key")
	}
}
