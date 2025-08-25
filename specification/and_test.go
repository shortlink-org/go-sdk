package specification

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// AndSpecificationTestSuite groups related AND specification tests
type AndSpecificationTestSuite struct {
	suite.Suite
	users []*TestUser
}

func (suite *AndSpecificationTestSuite) SetupTest() {
	suite.users = createTestUsers()
}

func TestAndSpecificationSuite(t *testing.T) {
	suite.Run(t, new(AndSpecificationTestSuite))
}

func (suite *AndSpecificationTestSuite) TestNewAndSpecification() {
	// Arrange
	spec1 := &UserAgeMinSpec{MinAge: 18}
	spec2 := &UserActiveSpec{}

	// Act
	andSpec := NewAndSpecification[TestUser](spec1, spec2)

	// Assert
	assert.NotNil(suite.T(), andSpec)
	assert.Len(suite.T(), andSpec.specs, 2)
	assert.Equal(suite.T(), spec1, andSpec.specs[0])
	assert.Equal(suite.T(), spec2, andSpec.specs[1])
}

func (suite *AndSpecificationTestSuite) TestNewAndSpecification_NoSpecs() {
	// Act
	andSpec := NewAndSpecification[TestUser]()

	// Assert
	assert.NotNil(suite.T(), andSpec)
	assert.Empty(suite.T(), andSpec.specs)
}

func (suite *AndSpecificationTestSuite) TestNewAndSpecification_SingleSpec() {
	// Arrange
	spec := &UserAgeMinSpec{MinAge: 18}

	// Act
	andSpec := NewAndSpecification[TestUser](spec)

	// Assert
	assert.NotNil(suite.T(), andSpec)
	assert.Len(suite.T(), andSpec.specs, 1)
	assert.Equal(suite.T(), spec, andSpec.specs[0])
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_AllPass() {
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	andSpec := NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err)
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_OneFails() {
	// Arrange - Bob is 17, so fails age requirement
	user := &TestUser{ID: 2, Name: "Bob", Age: 17, Email: "bob@example.com", IsActive: true}
	andSpec := NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "user age 17 is below minimum 18")
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_MultipleFail() {
	// Arrange - Charlie is inactive and has no email
	user := &TestUser{ID: 3, Name: "Charlie", Age: 30, Email: "", IsActive: false}
	andSpec := NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "user is not active")
	assert.Contains(suite.T(), err.Error(), "user email is empty")
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_EmptySpecs() {
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	andSpec := NewAndSpecification[TestUser]()

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err) // No specs to fail
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_NestedAndSpecifications() {
	// Arrange
	innerAnd1 := NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserAgeMaxSpec{MaxAge: 65},
	)
	innerAnd2 := NewAndSpecification[TestUser](
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)
	outerAnd := NewAndSpecification[TestUser](innerAnd1, innerAnd2)

	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}

	// Act
	err := outerAnd.IsSatisfiedBy(user)

	// Assert
	assert.NoError(suite.T(), err)
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_NestedAndSpecifications_Fails() {
	// Arrange
	innerAnd1 := NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserAgeMaxSpec{MaxAge: 65},
	)
	innerAnd2 := NewAndSpecification[TestUser](
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)
	outerAnd := NewAndSpecification[TestUser](innerAnd1, innerAnd2)

	// Bob is 17, so fails the age requirement in innerAnd1
	user := &TestUser{ID: 2, Name: "Bob", Age: 17, Email: "bob@example.com", IsActive: true}

	// Act
	err := outerAnd.IsSatisfiedBy(user)

	// Assert
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "user age 17 is below minimum 18")
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_ErrorAggregation() {
	// Arrange - User that fails multiple specs
	user := &TestUser{ID: 6, Name: "Frank", Age: 16, Email: "frank.invalid", IsActive: false}
	andSpec := NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	assert.Error(suite.T(), err)
	
	// Check that all three errors are included
	assert.Contains(suite.T(), err.Error(), "user age 16 is below minimum 18")
	assert.Contains(suite.T(), err.Error(), "user is not active")
	assert.Contains(suite.T(), err.Error(), "is missing @ symbol")
}

func (suite *AndSpecificationTestSuite) TestWithFilter_ComplexScenario() {
	// Arrange - find users who are active, 18+, and have valid email
	andSpec := NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)

	// Act
	result, err := Filter(suite.users, andSpec)

	// Assert
	assert.Error(suite.T(), err) // Some users will fail the criteria
	assert.NotEmpty(suite.T(), result)

	// Verify all returned users meet all criteria
	for _, user := range result {
		assert.GreaterOrEqual(suite.T(), user.Age, 18, "User %s should be 18+", user.Name)
		assert.True(suite.T(), user.IsActive, "User %s should be active", user.Name)
		assert.NotEmpty(suite.T(), user.Email, "User %s should have email", user.Name)
		assert.Contains(suite.T(), user.Email, "@", "User %s should have valid email", user.Name)
	}

	// Check specific users
	expectedUsers := []string{"Alice", "Eve"} // Only these meet all criteria
	assert.Len(suite.T(), result, len(expectedUsers))
	
	for _, expectedName := range expectedUsers {
		found := false
		for _, user := range result {
			if user.Name == expectedName {
				found = true
				break
			}
		}
		assert.True(suite.T(), found, "Expected to find user %s in results", expectedName)
	}
}

// Standalone tests for additional scenarios
func TestAndSpecification_WithAlwaysPassSpecs(t *testing.T) {
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: true}
	andSpec := NewAndSpecification[TestUser](
		&AlwaysPassSpec[TestUser]{},
		&AlwaysPassSpec[TestUser]{},
		&AlwaysPassSpec[TestUser]{},
	)

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(t, err)
}

func TestAndSpecification_WithAlwaysFailSpecs(t *testing.T) {
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: true}
	andSpec := NewAndSpecification[TestUser](
		&AlwaysFailSpec[TestUser]{Reason: "fail1"},
		&AlwaysFailSpec[TestUser]{Reason: "fail2"},
	)

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fail1")
	assert.Contains(t, err.Error(), "fail2")
}

func TestAndSpecification_MixedPassFail(t *testing.T) {
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: true}
	andSpec := NewAndSpecification[TestUser](
		&AlwaysPassSpec[TestUser]{},
		&AlwaysFailSpec[TestUser]{Reason: "expected failure"},
		&AlwaysPassSpec[TestUser]{},
	)

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected failure")
}

func TestAndSpecification_DirectStructUsage(t *testing.T) {
	// Test using the struct directly instead of constructor
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: true}
	andSpec := &AndSpecification[TestUser]{
		specs: []Specification[TestUser]{
			&UserAgeMinSpec{MinAge: 18},
			&UserActiveSpec{},
		},
	}

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	assert.NoError(t, err)
}

func TestAndSpecification_NilUser(t *testing.T) {
	// Arrange
	andSpec := NewAndSpecification[TestUser](&UserActiveSpec{})

	// Act & Assert
	assert.Panics(t, func() {
		andSpec.IsSatisfiedBy(nil)
	})
}