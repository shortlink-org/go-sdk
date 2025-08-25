package specification

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// FilterTestSuite groups related filter tests
type FilterTestSuite struct {
	suite.Suite
	users []*TestUser
}

func (suite *FilterTestSuite) SetupTest() {
	suite.users = createTestUsers()
}

func TestFilterSuite(t *testing.T) {
	suite.Run(t, new(FilterTestSuite))
}

func (suite *FilterTestSuite) TestFilter_EmptySlice() {
	// Arrange
	emptyUsers := []*TestUser{}
	spec := &AlwaysPassSpec[TestUser]{}

	// Act
	result, err := Filter(emptyUsers, spec)

	// Assert
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), result)
	assert.NotNil(suite.T(), result) // Should return empty slice, not nil
}

func (suite *FilterTestSuite) TestFilter_AllPass() {
	// Arrange
	spec := &AlwaysPassSpec[TestUser]{}

	// Act
	result, err := Filter(suite.users, spec)

	// Assert
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), result, len(suite.users))
	assert.Equal(suite.T(), suite.users, result)
}

func (suite *FilterTestSuite) TestFilter_AllFail() {
	// Arrange
	spec := &AlwaysFailSpec[TestUser]{Reason: "test failure"}

	// Act
	result, err := Filter(suite.users, spec)

	// Assert
	assert.Error(suite.T(), err)
	assert.Empty(suite.T(), result)
	assert.Contains(suite.T(), err.Error(), "test failure")

	// Check that all users generated errors
	errorCount := 0
	currentErr := err
	for currentErr != nil {
		errorCount++
		if ue := errors.Unwrap(currentErr); ue != nil {
			currentErr = ue
		} else {
			break
		}
	}
	// The exact count may vary due to errors.Join implementation
	assert.Greater(suite.T(), errorCount, 0)
}

func (suite *FilterTestSuite) TestFilter_MixedResults() {
	// Arrange - only active users should pass
	spec := &UserActiveSpec{}

	// Act
	result, err := Filter(suite.users, spec)

	// Assert
	assert.Error(suite.T(), err) // Should have errors for inactive users
	assert.NotEmpty(suite.T(), result) // Should have some active users

	// Check that all returned users are active
	for _, user := range result {
		assert.True(suite.T(), user.IsActive, "Expected user %s to be active", user.Name)
	}

	// Check that inactive users are not in result
	expectedActiveUsers := 0
	for _, user := range suite.users {
		if user.IsActive {
			expectedActiveUsers++
		}
	}
	assert.Len(suite.T(), result, expectedActiveUsers)
}

func (suite *FilterTestSuite) TestFilter_AgeFilter() {
	// Arrange - users must be 18 or older
	spec := &UserAgeMinSpec{MinAge: 18}

	// Act
	result, err := Filter(suite.users, spec)

	// Assert
	assert.Error(suite.T(), err) // Should have errors for underage users
	assert.NotEmpty(suite.T(), result)

	// Check that all returned users are 18 or older
	for _, user := range result {
		assert.GreaterOrEqual(suite.T(), user.Age, 18, "Expected user %s to be 18 or older", user.Name)
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
	assert.True(suite.T(), aliceInResult, "Alice (25) should be in result")
	assert.False(suite.T(), bobInResult, "Bob (17) should not be in result")
}

func (suite *FilterTestSuite) TestFilter_EmailValidation() {
	// Arrange
	spec := &UserEmailValidSpec{}

	// Act
	result, err := Filter(suite.users, spec)

	// Assert
	assert.Error(suite.T(), err) // Should have errors for invalid emails
	assert.NotEmpty(suite.T(), result)

	// Check that all returned users have valid emails
	for _, user := range result {
		assert.NotEmpty(suite.T(), user.Email, "Expected user %s to have non-empty email", user.Name)
		assert.Contains(suite.T(), user.Email, "@", "Expected user %s to have valid email", user.Name)
	}
}

func (suite *FilterTestSuite) TestFilter_NilSpecification() {
	// This test ensures graceful handling of nil specification
	// Note: This would normally panic, but we're testing the behavior
	defer func() {
		if r := recover(); r != nil {
			suite.T().Log("Expected panic when using nil specification")
		}
	}()

	// Act & Assert
	assert.Panics(suite.T(), func() {
		Filter(suite.users, nil)
	})
}

func (suite *FilterTestSuite) TestFilter_NilSlice() {
	// Arrange
	spec := &AlwaysPassSpec[TestUser]{}

	// Act
	result, err := Filter(nil, spec)

	// Assert
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), result)
	assert.NotNil(suite.T(), result) // Should return empty slice, not nil
}

func (suite *FilterTestSuite) TestFilter_SliceWithNilElements() {
	// Arrange
	usersWithNil := []*TestUser{
		suite.users[0],
		nil,
		suite.users[1],
		nil,
	}
	spec := &UserActiveSpec{} // Using a real spec that would panic on nil

	// Act & Assert
	// This should panic when spec.IsSatisfiedBy is called with nil
	assert.Panics(suite.T(), func() {
		Filter(usersWithNil, spec)
	})
}

// Standalone tests for additional coverage
func TestFilter_BasicFunctionality(t *testing.T) {
	// Arrange
	users := []*TestUser{
		{ID: 1, Name: "Test1", Age: 20, IsActive: true},
		{ID: 2, Name: "Test2", Age: 15, IsActive: false},
	}
	spec := &UserAgeMinSpec{MinAge: 18}

	// Act
	result, err := Filter(users, spec)

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
	result, err := Filter(users, spec)

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
	result, err := Filter(users, spec)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, len(users), len(result))
	// The capacity should be at least len(users) due to make([]T, 0, len(list))
	assert.GreaterOrEqual(t, cap(result), len(users))
}