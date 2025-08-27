package specification_test

import (
	"errors"
	"fmt"
)

// TestUser represents a simple user for testing purposes.
type TestUser struct {
	ID       int
	Name     string
	Age      int
	Email    string
	IsActive bool
}

// Mock specifications for testing.
type AlwaysPassSpec[T any] struct{}

func (a *AlwaysPassSpec[T]) IsSatisfiedBy(item *T) error {
	return nil
}

type AlwaysFailSpec[T any] struct {
	Reason string
}

func (a *AlwaysFailSpec[T]) IsSatisfiedBy(item *T) error {
	if a.Reason == "" {
		a.Reason = "always fails"
	}

	return errors.New(a.Reason)
}

// UserSpecifications for more realistic testing.
type UserAgeMinSpec struct {
	MinAge int
}

func (u *UserAgeMinSpec) IsSatisfiedBy(user *TestUser) error {
	if user.Age < u.MinAge {
		return fmt.Errorf("user age %d is below minimum %d", user.Age, u.MinAge)
	}

	return nil
}

type UserAgeMaxSpec struct {
	MaxAge int
}

func (u *UserAgeMaxSpec) IsSatisfiedBy(user *TestUser) error {
	if user.Age > u.MaxAge {
		return fmt.Errorf("user age %d is above maximum %d", user.Age, u.MaxAge)
	}

	return nil
}

type UserActiveSpec struct{}

func (u *UserActiveSpec) IsSatisfiedBy(user *TestUser) error {
	if !user.IsActive {
		return errors.New("user is not active")
	}

	return nil
}

type UserEmailValidSpec struct{}

func (u *UserEmailValidSpec) IsSatisfiedBy(user *TestUser) error {
	if user.Email == "" {
		return errors.New("user email is empty")
	}
	// Simple email validation for testing
	if len(user.Email) < 3 || user.Email[0] == '@' || user.Email[len(user.Email)-1] == '@' {
		return fmt.Errorf("user email %s is invalid", user.Email)
	}

	hasAt := false

	for _, c := range user.Email {
		if c == '@' {
			if hasAt {
				return fmt.Errorf("user email %s has multiple @ symbols", user.Email)
			}

			hasAt = true
		}
	}

	if !hasAt {
		return fmt.Errorf("user email %s is missing @ symbol", user.Email)
	}

	return nil
}

// Helper function to create test users.
func createTestUsers() []*TestUser {
	return []*TestUser{
		{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true},
		{ID: 2, Name: "Bob", Age: 17, Email: "bob@example.com", IsActive: true},
		{ID: 3, Name: "Charlie", Age: 30, Email: "charlie@example.com", IsActive: false},
		{ID: 4, Name: "Diana", Age: 22, Email: "", IsActive: true},
		{ID: 5, Name: "Eve", Age: 35, Email: "eve@example.com", IsActive: true},
		{ID: 6, Name: "Frank", Age: 16, Email: "frank.invalid", IsActive: true},
		{ID: 7, Name: "Grace", Age: 28, Email: "grace@example.com", IsActive: false},
		{ID: 8, Name: "Henry", Age: 45, Email: "henry@@example.com", IsActive: true},
	}
}
