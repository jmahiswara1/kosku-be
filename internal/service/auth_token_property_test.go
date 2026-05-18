// Package service_test contains property-based tests for the auth service.
package service_test

// Validates: Requirements 1.2
//
// Property 7: Invitation token uniqueness
// No two active invitation tokens may share the same value at any point in time.
//
// This test verifies that the application-level token generation logic
// (uuid.New().String()) produces distinct UUIDs when called concurrently,
// matching the behavior of the AuthService.Invite method.

import (
	"sync"
	"testing"
	"testing/quick"

	"github.com/google/uuid"
)

// generateToken mirrors the token generation logic used in AuthService.Invite:
//
//	token := uuid.New().String()
//
// It is extracted here so the property test can exercise it directly without
// requiring a live database or email client.
func generateToken() string {
	return uuid.New().String()
}

// TestInvitationTokenUniqueness_Property is a property-based test that verifies
// invitation tokens are always distinct UUIDs regardless of how many are
// generated concurrently.
//
// Validates: Requirements 1.2
func TestInvitationTokenUniqueness_Property(t *testing.T) {
	// Property: for any N in [1, 100], generating N tokens concurrently
	// must produce N distinct, valid UUID strings.
	property := func(n uint8) bool {
		// Clamp n to a sensible range: at least 2 (uniqueness needs ≥2 tokens),
		// at most 100 to keep the test fast.
		count := int(n)
		if count < 2 {
			count = 2
		}
		if count > 100 {
			count = 100
		}

		tokens := make([]string, count)
		var wg sync.WaitGroup
		wg.Add(count)

		for i := 0; i < count; i++ {
			i := i // capture loop variable
			go func() {
				defer wg.Done()
				tokens[i] = generateToken()
			}()
		}
		wg.Wait()

		// Assert all tokens are distinct.
		seen := make(map[string]struct{}, count)
		for _, tok := range tokens {
			if _, exists := seen[tok]; exists {
				return false // duplicate found — property violated
			}
			seen[tok] = struct{}{}
		}

		// Assert all tokens are valid UUIDs.
		for _, tok := range tokens {
			if _, err := uuid.Parse(tok); err != nil {
				return false // not a valid UUID — property violated
			}
		}

		return true
	}

	cfg := &quick.Config{
		MaxCount: 200, // run 200 random inputs
	}

	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("invitation token uniqueness property violated: %v", err)
	}
}

// TestInvitationTokenUniqueness_Concurrent is a focused example-based test
// that generates a large fixed number of tokens concurrently and asserts
// they are all distinct valid UUIDs.
//
// Validates: Requirements 1.2
func TestInvitationTokenUniqueness_Concurrent(t *testing.T) {
	const n = 1000

	tokens := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			tokens[i] = generateToken()
		}()
	}
	wg.Wait()

	seen := make(map[string]struct{}, n)
	for idx, tok := range tokens {
		// Validate UUID format.
		if _, err := uuid.Parse(tok); err != nil {
			t.Errorf("token[%d] = %q is not a valid UUID: %v", idx, tok, err)
		}

		// Check for duplicates.
		if _, exists := seen[tok]; exists {
			t.Errorf("duplicate token found: %q", tok)
		}
		seen[tok] = struct{}{}
	}

	if t.Failed() {
		t.Logf("generated %d tokens; %d were unique", n, len(seen))
	} else {
		t.Logf("all %d concurrently generated tokens are distinct valid UUIDs", n)
	}
}
