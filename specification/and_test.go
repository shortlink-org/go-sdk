package specification_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/shortlink-org/go-sdk/specification"
)

// AndSpecificationTestSuite groups related AND specification tests.
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
	andSpec := specification.NewAndSpecification[TestUser](spec1, spec2)

	// Assert
	suite.NotNil(andSpec)
	suite.Len(andSpec.Specs, 2)
	suite.Equal(spec1, andSpec.Specs[0])
	suite.Equal(spec2, andSpec.Specs[1])
}

func (suite *AndSpecificationTestSuite) TestNewAndSpecification_NoSpecs() {
	// Act
	andSpec := specification.NewAndSpecification[TestUser]()

	// Assert
	suite.NotNil(andSpec)
	suite.Empty(andSpec.Specs)
}

func (suite *AndSpecificationTestSuite) TestNewAndSpecification_SingleSpec() {
	// Arrange
	spec := &UserAgeMinSpec{MinAge: 18}

	// Act
	andSpec := specification.NewAndSpecification[TestUser](spec)

	// Assert
	suite.NotNil(andSpec)
	suite.Len(andSpec.Specs, 1)
	suite.Equal(spec, andSpec.Specs[0])
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_AllPass() {
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	andSpec := specification.NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	suite.NoError(err)
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_OneFails() {
	// Arrange - Bob is 17, so fails age requirement
	user := &TestUser{ID: 2, Name: "Bob", Age: 17, Email: "bob@example.com", IsActive: true}
	andSpec := specification.NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	suite.Require().Error(err)
	suite.Require().Contains(err.Error(), "user age 17 is below minimum 18")
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_MultipleFail() {
	// Arrange - Charlie is inactive and has no email
	user := &TestUser{ID: 3, Name: "Charlie", Age: 30, Email: "", IsActive: false}
	andSpec := specification.NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	suite.Require().Error(err)
	suite.Require().Contains(err.Error(), "user is not active")
	suite.Require().Contains(err.Error(), "user email is empty")
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_EmptySpecs() {
	// Arrange
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	andSpec := specification.NewAndSpecification[TestUser]()

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	suite.NoError(err) // No Specs to fail
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_NestedAndSpecifications() {
	// Arrange
	innerAnd1 := specification.NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserAgeMaxSpec{MaxAge: 65},
	)
	innerAnd2 := specification.NewAndSpecification[TestUser](
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)
	outerAnd := specification.NewAndSpecification[TestUser](innerAnd1, innerAnd2)

	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}

	// Act
	err := outerAnd.IsSatisfiedBy(user)

	// Assert
	suite.NoError(err)
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_NestedAndSpecifications_Fails() {
	// Arrange
	innerAnd1 := specification.NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserAgeMaxSpec{MaxAge: 65},
	)
	innerAnd2 := specification.NewAndSpecification[TestUser](
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)
	outerAnd := specification.NewAndSpecification[TestUser](innerAnd1, innerAnd2)

	// Bob is 17, so fails the age requirement in innerAnd1
	user := &TestUser{ID: 2, Name: "Bob", Age: 17, Email: "bob@example.com", IsActive: true}

	// Act
	err := outerAnd.IsSatisfiedBy(user)

	// Assert
	suite.Require().Error(err)
	suite.Require().Contains(err.Error(), "user age 17 is below minimum 18")
}

func (suite *AndSpecificationTestSuite) TestIsSatisfiedBy_ErrorAggregation() {
	// Arrange - User that fails multiple Specs
	user := &TestUser{ID: 6, Name: "Frank", Age: 16, Email: "frank.invalid", IsActive: false}
	andSpec := specification.NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)

	// Act
	err := andSpec.IsSatisfiedBy(user)

	// Assert
	suite.Require().Error(err)

	// Check that all three errors are included
	suite.Require().Contains(err.Error(), "user age 16 is below minimum 18")
	suite.Require().Contains(err.Error(), "user is not active")
	suite.Require().Contains(err.Error(), "is missing @ symbol")
}

func (suite *AndSpecificationTestSuite) TestWithFilter_ComplexScenario() {
	// Arrange - find users who are active, 18+, and have valid email
	andSpec := specification.NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)

	// Act
	result, err := specification.Filter(suite.users, andSpec)

	// Assert
	suite.Require().Error(err) // Some users will fail the criteria
	suite.Require().NotEmpty(result)

	// Verify all returned users meet all criteria
	for _, user := range result {
		suite.Require().GreaterOrEqual(user.Age, 18, "User %s should be 18+", user.Name)
		suite.Require().True(user.IsActive, "User %s should be active", user.Name)
		suite.Require().NotEmpty(user.Email, "User %s should have email", user.Name)
		suite.Require().Contains(user.Email, "@", "User %s should have valid email", user.Name)
	}

	// Check specific users
	expectedUsers := []string{"Alice", "Eve"} // Only these meet all criteria
	suite.Len(result, len(expectedUsers))

	for _, expectedName := range expectedUsers {
		found := false

		for _, user := range result {
			if user.Name == expectedName {
				found = true

				break
			}
		}

		suite.True(found, "Expected to find user %s in results", expectedName)
	}
}

// Standalone tests for additional scenarios.
func TestAndSpecification_WithAlwaysPassSpecs(t *testing.T) {
	// Arrange
	user := &TestUser{ID: 1, Name: "Test", Age: 20, IsActive: true}
	andSpec := specification.NewAndSpecification[TestUser](
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
	andSpec := specification.NewAndSpecification[TestUser](
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
	andSpec := specification.NewAndSpecification[TestUser](
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
	andSpec := &specification.AndSpecification[TestUser]{
		Specs: []specification.Specification[TestUser]{
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
	andSpec := specification.NewAndSpecification[TestUser](&UserActiveSpec{})

	// Act & Assert
	assert.Panics(t, func() {
		err := andSpec.IsSatisfiedBy(nil)
		assert.Error(t, err)
	})
}
