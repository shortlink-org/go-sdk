package specification_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/shortlink-org/go-sdk/specification"
)

// OrSpecificationTestSuite groups related OR specification tests.
type OrSpecificationTestSuite struct {
	suite.Suite

	users []*TestUser
}

func (suite *OrSpecificationTestSuite) SetupTest() {
	suite.users = createTestUsers()
}

func TestOrSpecificationSuite(t *testing.T) {
	suite.Run(t, new(OrSpecificationTestSuite))
}

func (suite *OrSpecificationTestSuite) TestNewOrSpecification() {
	// Arrange
	spec1 := &UserAgeMinSpec{MinAge: 18}
	spec2 := &UserActiveSpec{}

	// Act
	orSpec := specification.NewOrSpecification[TestUser](spec1, spec2)

	// Assert
	suite.NotNil(orSpec)
	suite.Len(orSpec.Specs, 2)
	suite.Equal(spec1, orSpec.Specs[0])
	suite.Equal(spec2, orSpec.Specs[1])
}

func (suite *OrSpecificationTestSuite) TestNewOrSpecification_NoSpecs() {
	// Act
	orSpec := specification.NewOrSpecification[TestUser]()

	// Assert
	suite.NotNil(orSpec)
	suite.Empty(orSpec.Specs)
}

func (suite *OrSpecificationTestSuite) TestNewOrSpecification_SingleSpec() {
	// Arrange
	spec := &UserAgeMinSpec{MinAge: 18}

	// Act
	orSpec := specification.NewOrSpecification[TestUser](spec)

	// Assert
	suite.NotNil(orSpec)
	suite.Len(orSpec.Specs, 1)
	suite.Equal(spec, orSpec.Specs[0])
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_FirstPasses() {
	// Arrange - Alice is active (first spec passes)
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	orSpec := specification.NewOrSpecification[TestUser](
		&UserActiveSpec{},            // This will pass
		&UserAgeMinSpec{MinAge: 100}, // This would fail
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	suite.NoError(err)
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_SecondPasses() {
	// Arrange - Charlie is inactive but meets age requirement
	user := &TestUser{ID: 3, Name: "Charlie", Age: 30, Email: "charlie@example.com", IsActive: false}
	orSpec := specification.NewOrSpecification[TestUser](
		&UserActiveSpec{},           // This will fail
		&UserAgeMinSpec{MinAge: 18}, // This will pass
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	suite.NoError(err)
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_LastPasses() {
	// Arrange - Test with multiple Specs where only the last one passes
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	orSpec := specification.NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 100}, // Fail
		&UserAgeMaxSpec{MaxAge: 10},  // Fail
		&UserActiveSpec{},            // Pass
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	suite.NoError(err)
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_AllFail() {
	// Arrange - Bob (17, active) fails both age requirements
	user := &TestUser{ID: 2, Name: "Bob", Age: 17, Email: "bob@example.com", IsActive: true}
	orSpec := specification.NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18}, // Fail - too young
		&UserAgeMaxSpec{MaxAge: 15}, // Fail - too old for max
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	suite.Require().Error(err)
	suite.Require().Contains(err.Error(), "user age 17 is below minimum 18")
	suite.Require().Contains(err.Error(), "user age 17 is above maximum 15")
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_EmptySpecs() {
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	orSpec := specification.NewOrSpecification[TestUser]()

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	suite.NoError(err) // No Specs to fail
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_MultiplePassing() {
	// Arrange - User that meets multiple criteria
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	orSpec := specification.NewOrSpecification[TestUser](
		&UserActiveSpec{},           // Pass
		&UserAgeMinSpec{MinAge: 18}, // Pass
		&UserEmailValidSpec{},       // Pass
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	suite.NoError(err) // Should return on first success
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_NestedOrSpecifications() {
	// Arrange
	innerOr1 := specification.NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 100}, // Fail
		&UserActiveSpec{},            // Pass for active users
	)
	innerOr2 := specification.NewOrSpecification[TestUser](
		&UserAgeMaxSpec{MaxAge: 10}, // Fail
		&UserEmailValidSpec{},       // Pass for users with valid email
	)
	outerOr := specification.NewOrSpecification[TestUser](innerOr1, innerOr2)

	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}

	// Act
	err := outerOr.IsSatisfiedBy(user)

	// Assert
	suite.NoError(err) // innerOr1 passes because user is active
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_NestedOrSpecifications_AllFail() {
	// Arrange
	innerOr1 := specification.NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 100}, // Fail
		&UserActiveSpec{},            // Fail for inactive users
	)
	innerOr2 := specification.NewOrSpecification[TestUser](
		&UserAgeMaxSpec{MaxAge: 10}, // Fail
		&UserEmailValidSpec{},       // Fail for users with empty email
	)
	outerOr := specification.NewOrSpecification[TestUser](innerOr1, innerOr2)

	// Diana has empty email and we want both inner ORs to fail
	user := &TestUser{ID: 4, Name: "Diana", Age: 22, Email: "", IsActive: false}

	// Act
	err := outerOr.IsSatisfiedBy(user)

	// Assert
	suite.Error(err)
}

func (suite *OrSpecificationTestSuite) TestWithFilter_InclusiveScenario() {
	// Arrange - find users who are either under 20 OR over 30
	orSpec := specification.NewOrSpecification[TestUser](
		&UserAgeMaxSpec{MaxAge: 19}, // Include young users
		&UserAgeMinSpec{MinAge: 31}, // Include older users
	)

	// Act
	result, err := specification.Filter(suite.users, orSpec)

	// Assert
	suite.Require().Error(err) // Some users will be in the 20-30 range and fail both
	suite.Require().NotEmpty(result)

	// Verify all returned users meet at least one criteria
	for _, user := range result {
		satisfiesYoung := user.Age <= 19
		satisfiesOld := user.Age >= 31
		suite.Require().True(satisfiesYoung || satisfiesOld,
			"User %s (age %d) should be either ≤19 or ≥31", user.Name, user.Age)
	}

	// Check specific users based on test data
	// Bob (17) should be included (≤19)
	// Eve (35) should be included (≥31)
	// Alice (25) should NOT be included (20-30 range)

	bobIncluded := false
	eveIncluded := false
	aliceIncluded := false

	for _, user := range result {
		switch user.Name {
		case "Bob":
			bobIncluded = true
		case "Eve":
			eveIncluded = true
		case "Alice":
			aliceIncluded = true
		}
	}

	suite.True(bobIncluded, "Bob (17) should be included")
	suite.True(eveIncluded, "Eve (35) should be included")
	suite.False(aliceIncluded, "Alice (25) should not be included")
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_ShortCircuit() {
	// This test verifies that OR specification returns immediately on first success
	// We can't easily test short-circuiting directly, but we can test the behavior
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}

	// Create a spec that would normally aggregate multiple errors
	// but should return nil immediately when the first spec passes
	orSpec := specification.NewOrSpecification[TestUser](
		&UserActiveSpec{}, // This passes first
		&AlwaysFailSpec[TestUser]{Reason: "should not be reached"},
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	suite.NoError(err)
	// If short-circuiting works, we shouldn't see the "should not be reached" error
}

// Standalone tests for additional scenarios.
func TestOrSpecification_WithAlwaysPassSpecs(t *testing.T) {
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: true}
	orSpec := specification.NewOrSpecification[TestUser](
		&AlwaysPassSpec[TestUser]{},
		&AlwaysFailSpec[TestUser]{Reason: "should not matter"},
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(t, err)
}

func TestOrSpecification_WithAlwaysFailSpecs(t *testing.T) {
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: true}
	orSpec := specification.NewOrSpecification[TestUser](
		&AlwaysFailSpec[TestUser]{Reason: "fail1"},
		&AlwaysFailSpec[TestUser]{Reason: "fail2"},
		&AlwaysFailSpec[TestUser]{Reason: "fail3"},
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fail1")
	assert.Contains(t, err.Error(), "fail2")
	assert.Contains(t, err.Error(), "fail3")
}

func TestOrSpecification_DirectStructUsage(t *testing.T) {
	// Test using the struct directly instead of constructor
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: true}
	orSpec := &specification.OrSpecification[TestUser]{
		Specs: []specification.Specification[TestUser]{
			&UserAgeMinSpec{MinAge: 100}, // Fail
			&UserActiveSpec{},            // Pass
		},
	}

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(t, err)
}

func TestOrSpecification_NilUser(t *testing.T) {
	// Arrange
	orSpec := specification.NewOrSpecification[TestUser](&UserActiveSpec{})

	// Act & Assert
	assert.Panics(t, func() {
		_ = orSpec.IsSatisfiedBy(nil)
	})
}

func TestOrSpecification_ErrorAccumulation(t *testing.T) {
	// Test that OR accumulates all errors when all Specs fail
	// Arrange
	user := &TestUser{ID: 6, Name: "Frank", Age: 16, Email: "frank.invalid", IsActive: false}
	orSpec := specification.NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18}, // Fail: too young
		&UserActiveSpec{},           // Fail: not active
		&UserEmailValidSpec{},       // Fail: invalid email
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user age 16 is below minimum 18")
	assert.Contains(t, err.Error(), "user is not active")
	assert.Contains(t, err.Error(), "is missing @ symbol")
}

func TestOrSpecification_ComplexLogic(t *testing.T) {
	// Test: (Active AND Email) OR (Age >= 30)
	// This uses nested AND within OR
	// Arrange
	andSpec := specification.NewAndSpecification[TestUser](
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)
	orSpec := specification.NewOrSpecification[TestUser](
		andSpec,
		&UserAgeMinSpec{MinAge: 30},
	)

	// Test cases
	testCases := []struct {
		name     string
		user     *TestUser
		expected bool
		reason   string
	}{
		{
			name:     "Active with valid email (young)",
			user:     &TestUser{Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true},
			expected: true,
			reason:   "Should pass: active AND valid email",
		},
		{
			name:     "Inactive but old enough",
			user:     &TestUser{Name: "Charlie", Age: 35, Email: "", IsActive: false},
			expected: true,
			reason:   "Should pass: age >= 30",
		},
		{
			name:     "Young, inactive, invalid email",
			user:     &TestUser{Name: "Bob", Age: 17, Email: "invalid", IsActive: false},
			expected: false,
			reason:   "Should fail: doesn't meet either condition",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Act
			err := orSpec.IsSatisfiedBy(testCase.user)

			// Assert
			if testCase.expected {
				assert.NoError(t, err, testCase.reason)
			} else {
				assert.Error(t, err, testCase.reason)
			}
		})
	}
}
