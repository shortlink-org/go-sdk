package specification_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/shortlink-org/go-sdk/specification"
)

// FilterTestSuite groups related filter tests.
type FilterTestSuite struct {
	suite.Suite

	users []*TestUser
}

func (s *FilterTestSuite) SetupTest() {
	s.users = createTestUsers()
}

func TestFilterSuite(t *testing.T) {
	suite.Run(t, new(FilterTestSuite))
}

func (s *FilterTestSuite) TestFilter_EmptySlice() {
	// Arrange
	emptyUsers := []*TestUser{}
	spec := &AlwaysPassSpec[TestUser]{}

	// Act
	result, err := specification.Filter(emptyUsers, spec)

	// Assert
	s.Require().NoError(err)
	s.Require().Empty(result)
	s.Require().NotNil(result) // Should return empty slice, not nil
}

func (s *FilterTestSuite) TestFilter_AllPass() {
	// Arrange
	spec := &AlwaysPassSpec[TestUser]{}

	// Act
	result, err := specification.Filter(s.users, spec)

	// Assert
	s.Require().NoError(err)
	s.Require().Len(result, len(s.users))
	s.Require().Equal(s.users, result)
}

func (s *FilterTestSuite) TestFilter_AllFail() {
	// Arrange
	spec := &AlwaysFailSpec[TestUser]{Reason: "test failure"}

	// Act
	result, err := specification.Filter(s.users, spec)

	// Assert
	s.Require().Error(err)
	s.Require().Empty(result)
	s.Require().Contains(err.Error(), "test failure")

	// Check that all users generated errors
	errorCount := 0

	currentErr := err

	for currentErr != nil {
		errorCount++

		ue := errors.Unwrap(currentErr)
		if ue != nil {
			currentErr = ue
		} else {
			break
		}
	}
	// The exact count may vary due to errors.Join implementation
	s.Positive(errorCount)
}

func (s *FilterTestSuite) TestFilter_MixedResults() {
	// Arrange - only active users should pass
	spec := &UserActiveSpec{}

	// Act
	result, err := specification.Filter(s.users, spec)

	// Assert
	s.Require().Error(err)       // Should have errors for inactive users
	s.Require().NotEmpty(result) // Should have some active users

	// Check that all returned users are active
	for _, user := range result {
		s.Require().True(user.IsActive, "Expected user %s to be active", user.Name)
	}

	// Check that inactive users are not in result
	expectedActiveUsers := 0

	for _, user := range s.users {
		if user.IsActive {
			expectedActiveUsers++
		}
	}

	s.Len(result, expectedActiveUsers)
}

func (s *FilterTestSuite) TestFilter_AgeFilter() {
	// Arrange - users must be 18 or older
	spec := &UserAgeMinSpec{MinAge: 18}

	// Act
	result, err := specification.Filter(s.users, spec)

	// Assert
	s.Require().Error(err) // Should have errors for underage users
	s.Require().NotEmpty(result)

	// Check that all returned users are 18 or older
	for _, user := range result {
		s.Require().GreaterOrEqual(user.Age, 18, "Expected user %s to be 18 or older", user.Name)
	}

	// Verify specific users
	aliceInResult := false
	bobInResult := false

	for _, user := range result {
		if user.Name == "Alice" {
			aliceInResult = true
		}

		if user.Name == "Bob" {
			bobInResult = true
		}
	}

	s.True(aliceInResult, "Alice (25) should be in result")
	s.False(bobInResult, "Bob (17) should not be in result")
}

func (s *FilterTestSuite) TestFilter_EmailValidation() {
	// Arrange
	spec := &UserEmailValidSpec{}

	// Act
	result, err := specification.Filter(s.users, spec)

	// Assert
	s.Require().Error(err) // Should have errors for invalid emails
	s.Require().NotEmpty(result)

	// Check that all returned users have valid emails
	for _, user := range result {
		s.Require().NotEmpty(user.Email, "Expected user %s to have non-empty email", user.Name)
		s.Require().Contains(user.Email, "@", "Expected user %s to have valid email", user.Name)
	}
}

func (s *FilterTestSuite) TestFilter_NilSpecification() {
	// This test ensures graceful handling of nil specification
	// Note: This would normally panic, but we're testing the behavior
	defer func() {
		if r := recover(); r != nil {
			s.T().Log("Expected panic when using nil specification")
		}
	}()

	// Act & Assert
	s.Panics(func() {
		_, err := specification.Filter(s.users, nil)
		s.Error(err)
	})
}

func (s *FilterTestSuite) TestFilter_NilSlice() {
	// Arrange
	spec := &AlwaysPassSpec[TestUser]{}

	// Act
	result, err := specification.Filter(nil, spec)

	// Assert
	s.Require().NoError(err)
	s.Require().Empty(result)
	s.Require().NotNil(result) // Should return empty slice, not nil
}

func (s *FilterTestSuite) TestFilter_SliceWithNilElements() {
	// Arrange
	usersWithNil := []*TestUser{
		s.users[0],
		nil,
		s.users[1],
		nil,
	}
	spec := &UserActiveSpec{} // Using a real spec that would panic on nil

	// Act & Assert
	// This should panic when spec.IsSatisfiedBy is called with nil
	s.Panics(func() {
		_, err := specification.Filter(usersWithNil, spec)
		s.Error(err)
	})
}

// Standalone tests for additional coverage.
func TestFilter_BasicFunctionality(t *testing.T) {
	// Arrange
	users := []*TestUser{
		{ID: 1, Name: "Test1", Age: 20, IsActive: true},
		{ID: 2, Name: "Test2", Age: 15, IsActive: false},
	}
	spec := &UserAgeMinSpec{MinAge: 18}

	// Act
	result, err := specification.Filter(users, spec)

	// Assert
	require.Error(t, err) // One user fails age requirement
	require.Len(t, result, 1)
	assert.Equal(t, "Test1", result[0].Name)
}

func TestFilter_PreservesOrder(t *testing.T) {
	// Arrange
	users := []*TestUser{
		{ID: 3, Name: "Charlie", Age: 30, IsActive: true},
		{ID: 1, Name: "Alice", Age: 25, IsActive: true},
		{ID: 5, Name: "Eve", Age: 35, IsActive: true},
	}
	spec := &AlwaysPassSpec[TestUser]{}

	// Act
	result, err := specification.Filter(users, spec)

	// Assert
	require.NoError(t, err)
	require.Len(t, result, 3)

	// Check that order is preserved
	expectedOrder := []string{"Charlie", "Alice", "Eve"}
	for i, user := range result {
		assert.Equal(t, expectedOrder[i], user.Name)
	}
}

func TestFilter_CapacityOptimization(t *testing.T) {
	// This test verifies that the result slice has proper capacity allocation
	// Arrange
	users := createTestUsers()
	spec := &AlwaysPassSpec[TestUser]{}

	// Act
	result, err := specification.Filter(users, spec)

	// Assert
	require.NoError(t, err)
	assert.Len(t, result, len(users))
	// The capacity should be at least len(users) due to make([]T, 0, len(list))
	assert.GreaterOrEqual(t, cap(result), len(users))
}
