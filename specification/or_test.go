package specification

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// OrSpecificationTestSuite groups related OR specification tests
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
	orSpec := NewOrSpecification[TestUser](spec1, spec2)

	// Assert
	assert.NotNil(suite.T(), orSpec)
	assert.Len(suite.T(), orSpec.specs, 2)
	assert.Equal(suite.T(), spec1, orSpec.specs[0])
	assert.Equal(suite.T(), spec2, orSpec.specs[1])
}

func (suite *OrSpecificationTestSuite) TestNewOrSpecification_NoSpecs() {
	// Act
	orSpec := NewOrSpecification[TestUser]()

	// Assert
	assert.NotNil(suite.T(), orSpec)
	assert.Empty(suite.T(), orSpec.specs)
}

func (suite *OrSpecificationTestSuite) TestNewOrSpecification_SingleSpec() {
	// Arrange
	spec := &UserAgeMinSpec{MinAge: 18}

	// Act
	orSpec := NewOrSpecification[TestUser](spec)

	// Assert
	assert.NotNil(suite.T(), orSpec)
	assert.Len(suite.T(), orSpec.specs, 1)
	assert.Equal(suite.T(), spec, orSpec.specs[0])
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_FirstPasses() {
	// Arrange - Alice is active (first spec passes)
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	orSpec := NewOrSpecification[TestUser](
		&UserActiveSpec{},              // This will pass
		&UserAgeMinSpec{MinAge: 100},   // This would fail
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err)
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_SecondPasses() {
	// Arrange - Charlie is inactive but meets age requirement
	user := &TestUser{ID: 3, Name: "Charlie", Age: 30, Email: "charlie@example.com", IsActive: false}
	orSpec := NewOrSpecification[TestUser](
		&UserActiveSpec{},              // This will fail
		&UserAgeMinSpec{MinAge: 18},    // This will pass
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err)
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_LastPasses() {
	// Arrange - Test with multiple specs where only the last one passes
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	orSpec := NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 100},   // Fail
		&UserAgeMaxSpec{MaxAge: 10},    // Fail
		&UserActiveSpec{},              // Pass
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err)
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_AllFail() {
	// Arrange - Bob (17, active) fails both age requirements
	user := &TestUser{ID: 2, Name: "Bob", Age: 17, Email: "bob@example.com", IsActive: true}
	orSpec := NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},    // Fail - too young
		&UserAgeMaxSpec{MaxAge: 15},    // Fail - too old for max
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "user age 17 is below minimum 18")
	assert.Contains(suite.T(), err.Error(), "user age 17 is above maximum 15")
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_EmptySpecs() {
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	orSpec := NewOrSpecification[TestUser]()

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err) // No specs to fail
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_MultiplePassing() {
	// Arrange - User that meets multiple criteria
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	orSpec := NewOrSpecification[TestUser](
		&UserActiveSpec{},              // Pass
		&UserAgeMinSpec{MinAge: 18},    // Pass
		&UserEmailValidSpec{},          // Pass
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err) // Should return on first success
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_NestedOrSpecifications() {
	// Arrange
	innerOr1 := NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 100}, // Fail
		&UserActiveSpec{},            // Pass for active users
	)
	innerOr2 := NewOrSpecification[TestUser](
		&UserAgeMaxSpec{MaxAge: 10},  // Fail
		&UserEmailValidSpec{},        // Pass for users with valid email
	)
	outerOr := NewOrSpecification[TestUser](innerOr1, innerOr2)

	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}

	// Act
	err := outerOr.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err) // innerOr1 passes because user is active
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_NestedOrSpecifications_AllFail() {
	// Arrange
	innerOr1 := NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 100}, // Fail
		&UserActiveSpec{},            // Fail for inactive users
	)
	innerOr2 := NewOrSpecification[TestUser](
		&UserAgeMaxSpec{MaxAge: 10},  // Fail
		&UserEmailValidSpec{},        // Fail for users with empty email
	)
	outerOr := NewOrSpecification[TestUser](innerOr1, innerOr2)

	// Diana has empty email and we want both inner ORs to fail
	user := &TestUser{ID: 4, Name: "Diana", Age: 22, Email: "", IsActive: false}

	// Act
	err := outerOr.IsSatisfiedBy(user)

	// Assert
	assert.Error(suite.T(), err)
}

func (suite *OrSpecificationTestSuite) TestWithFilter_InclusiveScenario() {
	// Arrange - find users who are either under 20 OR over 30
	orSpec := NewOrSpecification[TestUser](
		&UserAgeMaxSpec{MaxAge: 19}, // Include young users
		&UserAgeMinSpec{MinAge: 31}, // Include older users
	)

	// Act
	result, err := Filter(suite.users, orSpec)

	// Assert
	assert.Error(suite.T(), err) // Some users will be in the 20-30 range and fail both
	assert.NotEmpty(suite.T(), result)

	// Verify all returned users meet at least one criteria
	for _, user := range result {
		satisfiesYoung := user.Age <= 19
		satisfiesOld := user.Age >= 31
		assert.True(suite.T(), satisfiesYoung || satisfiesOld, 
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
	
	assert.True(suite.T(), bobIncluded, "Bob (17) should be included")
	assert.True(suite.T(), eveIncluded, "Eve (35) should be included") 
	assert.False(suite.T(), aliceIncluded, "Alice (25) should not be included")
}

func (suite *OrSpecificationTestSuite) TestIsSatisfiedBy_ShortCircuit() {
	// This test verifies that OR specification returns immediately on first success
	// We can't easily test short-circuiting directly, but we can test the behavior
	
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	
	// Create a spec that would normally aggregate multiple errors
	// but should return nil immediately when the first spec passes
	orSpec := NewOrSpecification[TestUser](
		&UserActiveSpec{},              // This passes first
		&AlwaysFailSpec[TestUser]{Reason: "should not be reached"},
	)

	// Act
	err := orSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err)
	// If short-circuiting works, we shouldn't see the "should not be reached" error
}

// Standalone tests for additional scenarios
func TestOrSpecification_WithAlwaysPassSpecs(t *testing.T) {
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: true}
	orSpec := NewOrSpecification[TestUser](
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
	orSpec := NewOrSpecification[TestUser](
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
	orSpec := &OrSpecification[TestUser]{
		specs: []Specification[TestUser]{
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
	orSpec := NewOrSpecification[TestUser](&UserActiveSpec{})

	// Act & Assert
	assert.Panics(t, func() {
		orSpec.IsSatisfiedBy(nil)
	})
}

func TestOrSpecification_ErrorAccumulation(t *testing.T) {
	// Test that OR accumulates all errors when all specs fail
	// Arrange
	user := &TestUser{ID: 6, Name: "Frank", Age: 16, Email: "frank.invalid", IsActive: false}
	orSpec := NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},    // Fail: too young
		&UserActiveSpec{},              // Fail: not active  
		&UserEmailValidSpec{},          // Fail: invalid email
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
	andSpec := NewAndSpecification[TestUser](
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)
	orSpec := NewOrSpecification[TestUser](
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			err := orSpec.IsSatisfiedBy(tc.user)

			// Assert
			if tc.expected {
				assert.NoError(t, err, tc.reason)
			} else {
				assert.Error(t, err, tc.reason)
			}
		})
	}
}