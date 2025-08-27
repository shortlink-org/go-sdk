package specification_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/shortlink-org/go-sdk/specification"
)

// NotSpecificationTestSuite groups related NOT specification tests.
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
	notSpec := specification.NewNotSpecification[TestUser](spec)

	// Assert
	suite.NotNil(notSpec)
	suite.Equal(spec, notSpec.Spec)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_InnerSpecPasses() {
	// Arrange - Alice is active, so NOT active should fail
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	notSpec := specification.NewNotSpecification[TestUser](&UserActiveSpec{})

	// Act
	err := notSpec.IsSatisfiedBy(user)

	// Assert
	suite.Require().Error(err)
	suite.Require().ErrorIs(err, specification.ErrNotSatisfied)
	suite.Require().Equal("specification not satisfied", err.Error())
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_InnerSpecFails() {
	// Arrange - Charlie is inactive, so NOT active should pass
	user := &TestUser{ID: 3, Name: "Charlie", Age: 30, IsActive: false}
	notSpec := specification.NewNotSpecification[TestUser](&UserActiveSpec{})

	// Act
	err := notSpec.IsSatisfiedBy(user)

	// Assert
	suite.Require().NoError(err)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_WithAlwaysPass() {
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	notSpec := specification.NewNotSpecification[TestUser](&AlwaysPassSpec[TestUser]{})

	// Act
	err := notSpec.IsSatisfiedBy(user)

	// Assert
	suite.Require().Error(err)
	suite.Require().ErrorIs(err, specification.ErrNotSatisfied)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_WithAlwaysFail() {
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	notSpec := specification.NewNotSpecification[TestUser](&AlwaysFailSpec[TestUser]{Reason: "test failure"})

	// Act
	err := notSpec.IsSatisfiedBy(user)

	// Assert
	suite.Require().NoError(err) // NOT(fail) = pass
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_AgeSpecification() {
	// Test NOT age specification
	// Arrange
	youngUser := &TestUser{ID: 2, Name: "Bob", Age: 17, IsActive: true}
	oldUser := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}

	// NOT (age >= 18) should pass for users under 18
	notAdultSpec := specification.NewNotSpecification[TestUser](&UserAgeMinSpec{MinAge: 18})

	// Act & Assert for young user (should pass NOT adult)
	err := notAdultSpec.IsSatisfiedBy(youngUser)
	suite.Require().NoError(err)

	// Act & Assert for adult user (should fail NOT adult)
	err = notAdultSpec.IsSatisfiedBy(oldUser)
	suite.Require().Error(err)
	suite.Require().ErrorIs(err, specification.ErrNotSatisfied)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_EmailSpecification() {
	// Test NOT email specification
	// Arrange
	userWithEmail := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	userWithoutEmail := &TestUser{ID: 4, Name: "Diana", Age: 22, Email: "", IsActive: true}

	// NOT (valid email) should pass for users without valid email
	notEmailSpec := specification.NewNotSpecification[TestUser](&UserEmailValidSpec{})

	// Act & Assert for user with email (should fail NOT email)
	err := notEmailSpec.IsSatisfiedBy(userWithEmail)
	suite.Require().Error(err)
	suite.Require().ErrorIs(err, specification.ErrNotSatisfied)

	// Act & Assert for user without email (should pass NOT email)
	err = notEmailSpec.IsSatisfiedBy(userWithoutEmail)
	suite.Require().NoError(err)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_NestedNotSpecifications() {
	// Test NOT(NOT(spec)) = spec
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}

	innerNotSpec := specification.NewNotSpecification[TestUser](&UserActiveSpec{})
	doubleNotSpec := specification.NewNotSpecification[TestUser](innerNotSpec)

	// Act
	err := doubleNotSpec.IsSatisfiedBy(user)

	// Assert
	// Alice is active, so:
	// UserActiveSpec(Alice) = nil (pass)
	// NOT(UserActiveSpec(Alice)) = ErrNotSatisfied (fail)
	// NOT(NOT(UserActiveSpec(Alice))) = nil (pass)
	suite.Require().NoError(err)
}

func (suite *NotSpecificationTestSuite) TestIsSatisfiedBy_NestedNotSpecifications_Inactive() {
	// Test NOT(NOT(spec)) = spec with inactive user
	// Arrange
	user := &TestUser{ID: 3, Name: "Charlie", Age: 30, IsActive: false}

	innerNotSpec := specification.NewNotSpecification[TestUser](&UserActiveSpec{})
	doubleNotSpec := specification.NewNotSpecification[TestUser](innerNotSpec)

	// Act
	err := doubleNotSpec.IsSatisfiedBy(user)

	// Assert
	// Charlie is inactive, so:
	// UserActiveSpec(Charlie) = error (fail)
	// NOT(UserActiveSpec(Charlie)) = nil (pass)
	// NOT(NOT(UserActiveSpec(Charlie))) = ErrNotSatisfied (fail)
	suite.Require().Error(err)
	suite.Require().ErrorIs(err, specification.ErrNotSatisfied)
}

func (suite *NotSpecificationTestSuite) TestWithFilter_FindInactiveUsers() {
	// Arrange - find users who are NOT active
	notActiveSpec := specification.NewNotSpecification[TestUser](&UserActiveSpec{})

	// Act
	result, err := specification.Filter(suite.users, notActiveSpec)

	// Assert
	suite.Require().Error(err) // Active users will fail the NOT active spec
	suite.NotEmpty(result)

	// Verify all returned users are inactive
	for _, user := range result {
		suite.False(user.IsActive, "User %s should be inactive", user.Name)
	}

	// Check specific users - Charlie and Grace should be in results (they're inactive)
	expectedInactiveUsers := []string{"Charlie", "Grace"}
	suite.Len(result, len(expectedInactiveUsers))

	for _, expectedName := range expectedInactiveUsers {
		found := false

		for _, user := range result {
			if user.Name == expectedName {
				found = true

				break
			}
		}

		suite.True(found, "Expected to find inactive user %s in results", expectedName)
	}
}

func (suite *NotSpecificationTestSuite) TestWithFilter_FindUsersWithoutValidEmail() {
	// Arrange - find users who do NOT have valid email
	notValidEmailSpec := specification.NewNotSpecification[TestUser](&UserEmailValidSpec{})

	// Act
	result, err := specification.Filter(suite.users, notValidEmailSpec)

	// Assert
	suite.Require().Error(err) // Users with valid email will fail the NOT valid email spec
	suite.NotEmpty(result)

	// Verify all returned users don't have valid email
	for _, user := range result {
		emailSpec := &UserEmailValidSpec{}
		emailErr := emailSpec.IsSatisfiedBy(user)
		suite.Require().Error(emailErr, "User %s should not have valid email", user.Name)
	}
}

func (suite *NotSpecificationTestSuite) TestWithComplexSpecifications() {
	// Test NOT with AND specification
	// Find users who are NOT (active AND adult)
	// This should include: inactive users OR users under 18
	// Arrange
	activeAndAdultSpec := specification.NewAndSpecification[TestUser](
		&UserActiveSpec{},
		&UserAgeMinSpec{MinAge: 18},
	)
	notActiveAndAdultSpec := specification.NewNotSpecification[TestUser](activeAndAdultSpec)

	// Act
	result, err := specification.Filter(suite.users, notActiveAndAdultSpec)

	// Assert
	suite.Require().Error(err) // Some users are active AND adult
	suite.NotEmpty(result)

	// Verify the logic: users should be either inactive OR under 18 (or both)
	for _, user := range result {
		isNotActiveOrNotAdult := !user.IsActive || user.Age < 18
		suite.True(isNotActiveOrNotAdult,
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

	suite.True(bobIncluded, "Bob should be included (under 18)")
	suite.True(charlieIncluded, "Charlie should be included (inactive)")
	suite.False(aliceIncluded, "Alice should not be included (active AND adult)")
}

// Standalone tests for additional scenarios.
func TestNotSpecification_ErrNotSatisfiedConstant(t *testing.T) {
	// Test that ErrNotSatisfied is properly defined
	require.Error(t, specification.ErrNotSatisfied)
	require.Equal(t, "specification not satisfied", specification.ErrNotSatisfied.Error())
	// removed: assert.ErrorIs(t, specification.ErrNotSatisfied, specification.ErrNotSatisfied)
}

func TestNotSpecification_DirectStructUsage(t *testing.T) {
	// Test using the struct directly instead of constructor
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: false}
	notSpec := &specification.NotSpecification[TestUser]{
		Spec: &UserActiveSpec{},
	}

	// Act
	err := notSpec.IsSatisfiedBy(user)

	// Assert
	require.NoError(t, err) // User is inactive, so NOT active passes
}

func TestNotSpecification_NilUser(t *testing.T) {
	// Arrange
	notSpec := specification.NewNotSpecification[TestUser](&UserActiveSpec{})

	// Act & Assert
	require.Panics(t, func() {
		_ = notSpec.IsSatisfiedBy(nil)
	})
}

func TestNotSpecification_WithOrSpecification(t *testing.T) {
	// Test NOT with OR specification
	// NOT (active OR adult) = NOT active AND NOT adult = inactive AND under 18
	// Arrange
	activeOrAdultSpec := specification.NewOrSpecification[TestUser](
		&UserActiveSpec{},
		&UserAgeMinSpec{MinAge: 18},
	)
	notActiveOrAdultSpec := specification.NewNotSpecification[TestUser](activeOrAdultSpec)

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

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Act
			err := notActiveOrAdultSpec.IsSatisfiedBy(testCase.user)

			// Assert
			if testCase.expected {
				require.NoError(t, err, testCase.reason)
			} else {
				require.Error(t, err, testCase.reason)
				require.ErrorIs(t, err, specification.ErrNotSatisfied)
			}
		})
	}
}

func TestNotSpecification_ChainedNots(t *testing.T) {
	// Test multiple levels of NOT
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: true}

	// NOT(NOT(NOT(active))) = NOT(active)
	level1 := specification.NewNotSpecification[TestUser](&UserActiveSpec{}) // NOT active
	level2 := specification.NewNotSpecification[TestUser](level1)            // NOT active = active
	level3 := specification.NewNotSpecification[TestUser](level2)            // NOT active = NOT active

	// Act
	err1 := level1.IsSatisfiedBy(user) // Should fail (user is active)
	err2 := level2.IsSatisfiedBy(user) // Should pass (user is active)
	err3 := level3.IsSatisfiedBy(user) // Should fail (user is active)

	// Assert
	require.Error(t, err1)
	require.ErrorIs(t, err1, specification.ErrNotSatisfied)
	require.NoError(t, err2)

	require.Error(t, err3)
	require.ErrorIs(t, err3, specification.ErrNotSatisfied)
}

func TestNotSpecification_PreservesInnerError(t *testing.T) {
	// NOT specification should not preserve the inner error when inner spec fails
	// because it converts failure to success
	// Arrange
	user := &TestUser{ID: 2, Name: "Bob", Age: 17, IsActive: true}
	notAgeSpec := specification.NewNotSpecification[TestUser](&UserAgeMinSpec{MinAge: 18})

	// Act
	err := notAgeSpec.IsSatisfiedBy(user)

	// Assert
	require.NoError(t, err) // NOT(age fail) = pass, no error preserved
}

func TestNotSpecification_DiscardsPreviousError(t *testing.T) {
	// When inner spec passes, NOT should return ErrNotSatisfied, not the absence of inner error
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	notAgeSpec := specification.NewNotSpecification[TestUser](&UserAgeMinSpec{MinAge: 18})

	// Act
	err := notAgeSpec.IsSatisfiedBy(user)

	// Assert
	require.Error(t, err)
	require.ErrorIs(t, err, specification.ErrNotSatisfied)
	assert.NotContains(t, err.Error(), "age") // Should not contain inner spec's success message
}
