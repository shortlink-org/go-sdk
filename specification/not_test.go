package specification

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// NotSpecificationTestSuite groups related NOT specification tests
type NotSpecificationTestSuite struct {
	suite.Suite
	users []*TestUser
}

func (suite *NotSpecificationTestSuite) SetupTest() {
	suite.users = createTestUsers()
}

func TestNotSpecificationSuite(t *testing.T) {
	suite.Run(t, new(NotSpecificationTestSuite))
}

func (suite *NotSpecificationTestSuite) TestNewNotSpecification() {
	// Arrange
	spec := &UserActiveSpec{}

	// Act
	notSpec := NewNotSpecification[TestUser](spec)

	// Assert
	assert.NotNil(suite.T(), notSpec)
	assert.Equal(suite.T(), spec, notSpec.spec)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_InnerSpecPasses() {
	// Arrange - Alice is active, so NOT active should fail
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	notSpec := NewNotSpecification[TestUser](&UserActiveSpec{})

	// Act
	err := notSpec.IsSatisfiedBy(user)

	// Assert
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrNotSatisfied)
	assert.Equal(suite.T(), "specification not satisfied", err.Error())
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_InnerSpecFails() {
	// Arrange - Charlie is inactive, so NOT active should pass
	user := &TestUser{ID: 3, Name: "Charlie", Age: 30, Email: "charlie@example.com", IsActive: false}
	notSpec := NewNotSpecification[TestUser](&UserActiveSpec{})

	// Act
	err := notSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_WithAlwaysPass() {
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	notSpec := NewNotSpecification[TestUser](&AlwaysPassSpec[TestUser]{})

	// Act
	err := notSpec.IsSatisfiedBy(user)

	// Assert
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrNotSatisfied)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_WithAlwaysFail() {
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	notSpec := NewNotSpecification[TestUser](&AlwaysFailSpec[TestUser]{Reason: "test failure"})

	// Act
	err := notSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err) // NOT(fail) = pass
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_AgeSpecification() {
	// Test NOT age specification
	// Arrange
	youngUser := &TestUser{ID: 2, Name: "Bob", Age: 17, IsActive: true}
	oldUser := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	
	// NOT (age >= 18) should pass for users under 18
	notAdultSpec := NewNotSpecification[TestUser](&UserAgeMinSpec{MinAge: 18})

	// Act & Assert for young user (should pass NOT adult)
	err := notAdultSpec.IsSatisfiedBy(youngUser)
	assert.NoError(suite.T(), err)

	// Act & Assert for adult user (should fail NOT adult) 
	err = notAdultSpec.IsSatisfiedBy(oldUser)
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrNotSatisfied)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_EmailSpecification() {
	// Test NOT email specification
	// Arrange
	userWithEmail := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	userWithoutEmail := &TestUser{ID: 4, Name: "Diana", Age: 22, Email: "", IsActive: true}
	
	// NOT (valid email) should pass for users without valid email
	notEmailSpec := NewNotSpecification[TestUser](&UserEmailValidSpec{})

	// Act & Assert for user with email (should fail NOT email)
	err := notEmailSpec.IsSatisfiedBy(userWithEmail)
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrNotSatisfied)

	// Act & Assert for user without email (should pass NOT email)
	err = notEmailSpec.IsSatisfiedBy(userWithoutEmail)
	assert.NoError(suite.T(), err)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_NestedNotSpecifications() {
	// Test NOT(NOT(spec)) = spec
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	
	innerNotSpec := NewNotSpecification[TestUser](&UserActiveSpec{})
	doubleNotSpec := NewNotSpecification[TestUser](innerNotSpec)

	// Act
	err := doubleNotSpec.IsSatisfiedBy(user)

	// Assert
	// Alice is active, so:
	// UserActiveSpec(Alice) = nil (pass)
	// NOT(UserActiveSpec(Alice)) = ErrNotSatisfied (fail)  
	// NOT(NOT(UserActiveSpec(Alice))) = nil (pass)
	assert.NoError(suite.T(), err)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_NestedNotSpecifications_Inactive() {
	// Test NOT(NOT(spec)) = spec with inactive user
	// Arrange
	user := &TestUser{ID: 3, Name: "Charlie", Age: 30, IsActive: false}
	
	innerNotSpec := NewNotSpecification[TestUser](&UserActiveSpec{})
	doubleNotSpec := NewNotSpecification[TestUser](innerNotSpec)

	// Act
	err := doubleNotSpec.IsSatisfiedBy(user)

	// Assert
	// Charlie is inactive, so:
	// UserActiveSpec(Charlie) = error (fail)
	// NOT(UserActiveSpec(Charlie)) = nil (pass)
	// NOT(NOT(UserActiveSpec(Charlie))) = ErrNotSatisfied (fail)
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrNotSatisfied)
}

func (suite *NotSpecificationTestSuite) TestWithFilter_FindInactiveUsers() {
	// Arrange - find users who are NOT active
	notActiveSpec := NewNotSpecification[TestUser](&UserActiveSpec{})

	// Act
	result, err := Filter(suite.users, notActiveSpec)

	// Assert
	assert.Error(suite.T(), err) // Active users will fail the NOT active spec
	assert.NotEmpty(suite.T(), result)

	// Verify all returned users are inactive
	for _, user := range result {
		assert.False(suite.T(), user.IsActive, "User %s should be inactive", user.Name)
	}

	// Check specific users - Charlie and Grace should be in results (they're inactive)
	expectedInactiveUsers := []string{"Charlie", "Grace"}
	assert.Len(suite.T(), result, len(expectedInactiveUsers))
	
	for _, expectedName := range expectedInactiveUsers {
		found := false
		for _, user := range result {
			if user.Name == expectedName {
				found = true
				break
			}
		}
		assert.True(suite.T(), found, "Expected to find inactive user %s in results", expectedName)
	}
}

func (suite *NotSpecificationTestSuite) TestWithFilter_FindUsersWithoutValidEmail() {
	// Arrange - find users who do NOT have valid email
	notValidEmailSpec := NewNotSpecification[TestUser](&UserEmailValidSpec{})

	// Act
	result, err := Filter(suite.users, notValidEmailSpec)

	// Assert
	assert.Error(suite.T(), err) // Users with valid email will fail the NOT valid email spec
	assert.NotEmpty(suite.T(), result)

	// Verify all returned users don't have valid email
	for _, user := range result {
		emailSpec := &UserEmailValidSpec{}
		emailErr := emailSpec.IsSatisfiedBy(user)
		assert.Error(suite.T(), emailErr, "User %s should not have valid email", user.Name)
	}
}

func (suite *NotSpecificationTestSuite) TestWithComplexSpecifications() {
	// Test NOT with AND specification
	// Find users who are NOT (active AND adult)
	// This should include: inactive users OR users under 18
	
	// Arrange
	activeAndAdultSpec := NewAndSpecification[TestUser](
		&UserActiveSpec{},
		&UserAgeMinSpec{MinAge: 18},
	)
	notActiveAndAdultSpec := NewNotSpecification[TestUser](activeAndAdultSpec)

	// Act
	result, err := Filter(suite.users, notActiveAndAdultSpec)

	// Assert
	assert.Error(suite.T(), err) // Some users are active AND adult
	assert.NotEmpty(suite.T(), result)

	// Verify the logic: users should be either inactive OR under 18 (or both)
	for _, user := range result {
		isNotActiveOrNotAdult := !user.IsActive || user.Age < 18
		assert.True(suite.T(), isNotActiveOrNotAdult, 
			"User %s should be either inactive or under 18", user.Name)
	}

	// Check specific users:
	// Bob (17, active) - should be included (under 18)
	// Charlie (30, inactive) - should be included (inactive)
	// Alice (25, active) - should NOT be included (active AND adult)
	
	bobIncluded := false
	charlieIncluded := false
	aliceIncluded := false
	
	for _, user := range result {
		switch user.Name {
		case "Bob":
			bobIncluded = true
		case "Charlie":
			charlieIncluded = true
		case "Alice":
			aliceIncluded = true
		}
	}
	
	assert.True(suite.T(), bobIncluded, "Bob should be included (under 18)")
	assert.True(suite.T(), charlieIncluded, "Charlie should be included (inactive)")
	assert.False(suite.T(), aliceIncluded, "Alice should not be included (active AND adult)")
}

// Standalone tests for additional scenarios
func TestNotSpecification_ErrNotSatisfiedConstant(t *testing.T) {
	// Test that ErrNotSatisfied is properly defined
	assert.NotNil(t, ErrNotSatisfied)
	assert.Equal(t, "specification not satisfied", ErrNotSatisfied.Error())
	assert.True(t, errors.Is(ErrNotSatisfied, ErrNotSatisfied))
}

func TestNotSpecification_DirectStructUsage(t *testing.T) {
	// Test using the struct directly instead of constructor
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: false}
	notSpec := &NotSpecification[TestUser]{
		spec: &UserActiveSpec{},
	}

	// Act
	err := notSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(t, err) // User is inactive, so NOT active passes
}

func TestNotSpecification_NilUser(t *testing.T) {
	// Arrange
	notSpec := NewNotSpecification[TestUser](&UserActiveSpec{})

	// Act & Assert
	assert.Panics(t, func() {
		notSpec.IsSatisfiedBy(nil)
	})
}

func TestNotSpecification_WithOrSpecification(t *testing.T) {
	// Test NOT with OR specification
	// NOT (active OR adult) = NOT active AND NOT adult = inactive AND under 18
	
	// Arrange
	activeOrAdultSpec := NewOrSpecification[TestUser](
		&UserActiveSpec{},
		&UserAgeMinSpec{MinAge: 18},
	)
	notActiveOrAdultSpec := NewNotSpecification[TestUser](activeOrAdultSpec)

	testCases := []struct {
		name     string
		user     *TestUser
		expected bool
		reason   string
	}{
		{
			name:     "Active adult",
			user:     &TestUser{Name: "Alice", Age: 25, IsActive: true},
			expected: false,
			reason:   "Should fail: active OR adult is true",
		},
		{
			name:     "Inactive adult", 
			user:     &TestUser{Name: "Charlie", Age: 30, IsActive: false},
			expected: false,
			reason:   "Should fail: inactive but adult (adult part makes OR true)",
		},
		{
			name:     "Active minor",
			user:     &TestUser{Name: "Bob", Age: 17, IsActive: true},
			expected: false,
			reason:   "Should fail: minor but active (active part makes OR true)",
		},
		{
			name:     "Inactive minor",
			user:     &TestUser{Name: "YoungInactive", Age: 16, IsActive: false},
			expected: true,
			reason:   "Should pass: neither active nor adult",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			err := notActiveOrAdultSpec.IsSatisfiedBy(tc.user)

			// Assert
			if tc.expected {
				assert.NoError(t, err, tc.reason)
			} else {
				assert.Error(t, err, tc.reason)
				assert.ErrorIs(t, err, ErrNotSatisfied)
			}
		})
	}
}

func TestNotSpecification_ChainedNots(t *testing.T) {
	// Test multiple levels of NOT
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: true}
	
	// NOT(NOT(NOT(active))) = NOT(active)
	level1 := NewNotSpecification[TestUser](&UserActiveSpec{})     // NOT active
	level2 := NewNotSpecification[TestUser](level1)                // NOT NOT active = active
	level3 := NewNotSpecification[TestUser](level2)                // NOT NOT NOT active = NOT active

	// Act
	err1 := level1.IsSatisfiedBy(user) // Should fail (user is active)
	err2 := level2.IsSatisfiedBy(user) // Should pass (user is active)
	err3 := level3.IsSatisfiedBy(user) // Should fail (user is active)

	// Assert
	assert.Error(t, err1)
	assert.ErrorIs(t, err1, ErrNotSatisfied)
	
	assert.NoError(t, err2)
	
	assert.Error(t, err3)
	assert.ErrorIs(t, err3, ErrNotSatisfied)
}

func TestNotSpecification_PreservesInnerError(t *testing.T) {
	// NOT specification should not preserve the inner error when inner spec fails
	// because it converts failure to success
	
	// Arrange
	user := &TestUser{ID: 2, Name: "Bob", Age: 17, IsActive: true}
	notAgeSpec := NewNotSpecification[TestUser](&UserAgeMinSpec{MinAge: 18})

	// Act
	err := notAgeSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(t, err) // NOT(age fail) = pass, no error preserved
}

func TestNotSpecification_DiscardsPreviousError(t *testing.T) {
	// When inner spec passes, NOT should return ErrNotSatisfied, not the absence of inner error
	
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	notAgeSpec := NewNotSpecification[TestUser](&UserAgeMinSpec{MinAge: 18})

	// Act
	err := notAgeSpec.IsSatisfiedBy(user)

	// Assert
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotSatisfied)
	assert.NotContains(t, err.Error(), "age") // Should not contain inner spec's success message
}