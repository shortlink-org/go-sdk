package specification

import (
	"testing"
)

// Fuzz test for Filter function to discover edge cases
func FuzzFilter(f *testing.F) {
	// Seed with some initial test cases
	f.Add(int8(25), true, "alice@example.com")
	f.Add(int8(17), false, "")
	f.Add(int8(35), true, "invalid.email")

	f.Fuzz(func(t *testing.T, age int8, isActive bool, email string) {
		// Skip invalid ages to focus on business logic
		if age < 0 || age > 120 {
			t.Skip()
		}

		user := &TestUser{
			ID:       1,
			Name:     "FuzzUser",
			Age:      int(age),
			Email:    email,
			IsActive: isActive,
		}

		users := []*TestUser{user}
		spec := &UserActiveSpec{}

		// This should never panic
		result, err := Filter(users, spec)

		// Basic invariants that should always hold
		if result == nil {
			t.Error("Filter should never return nil slice")
		}

		if len(result) > len(users) {
			t.Error("Result cannot have more items than input")
		}

		// If user is active, should be in result (no error for this user)
		// If user is inactive, should not be in result (error expected)
		if isActive && len(result) == 0 {
			t.Error("Active user should be in result")
		}
		if !isActive && err == nil {
			t.Error("Inactive user should generate error")
		}
	})
}

// Fuzz test for AndSpecification
func FuzzAndSpecification(f *testing.F) {
	f.Add(int8(25), int8(18), int8(65))

	f.Fuzz(func(t *testing.T, userAge, minAge, maxAge int8) {
		if userAge < 0 || userAge > 120 || minAge < 0 || maxAge < 0 || minAge > maxAge {
			t.Skip()
		}

		user := &TestUser{Age: int(userAge), IsActive: true}
		
		andSpec := NewAndSpecification[TestUser](
			&UserAgeMinSpec{MinAge: int(minAge)},
			&UserAgeMaxSpec{MaxAge: int(maxAge)},
		)

		// Should never panic
		err := andSpec.IsSatisfiedBy(user)

		// Verify logical consistency
		shouldPass := int(userAge) >= int(minAge) && int(userAge) <= int(maxAge)
		actuallyPassed := err == nil

		if shouldPass != actuallyPassed {
			t.Errorf("Logic inconsistency: user age %d, min %d, max %d, should pass: %v, actually passed: %v",
				userAge, minAge, maxAge, shouldPass, actuallyPassed)
		}
	})
}