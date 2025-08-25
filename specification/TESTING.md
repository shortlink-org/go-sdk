# Testing Documentation

## Overview

This directory contains comprehensive test cases for the specification pattern implementation. The test suite achieves **100% code coverage** and follows Go testing best practices using the testify framework.

## Test Files

### Core Test Files

- **`test_helpers_test.go`** - Mock specifications and test data structures
- **`filter_test.go`** - Tests for the Filter function 
- **`and_test.go`** - Tests for AndSpecification
- **`or_test.go`** - Tests for OrSpecification  
- **`not_test.go`** - Tests for NotSpecification
- **`benchmark_test.go`** - Performance benchmarks and regression tests

## Test Coverage

```bash
# Run tests with coverage
go test ./specification -cover

# Generate detailed coverage report
go test ./specification -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

**Current Coverage: 100.0% of statements**

## Test Structure

### Test Suites
Uses testify's `suite.Suite` for organizing related tests:
- `FilterTestSuite` - Filter function tests
- `AndSpecificationTestSuite` - AND specification tests  
- `OrSpecificationTestSuite` - OR specification tests
- `NotSpecificationTestSuite` - NOT specification tests

### Test Data
- `TestUser` struct - Realistic domain object for testing
- Mock specifications (`AlwaysPassSpec`, `AlwaysFailSpec`)
- Domain-specific specs (`UserActiveSpec`, `UserAgeMinSpec`, etc.)

## Key Test Scenarios

### Filter Function
- ✅ Empty slice handling
- ✅ All pass/fail scenarios
- ✅ Mixed results with error aggregation
- ✅ Nil handling and edge cases
- ✅ Order preservation
- ✅ Capacity optimization

### AND Specification
- ✅ All specifications must pass
- ✅ Error aggregation from multiple failures
- ✅ Nested AND specifications
- ✅ Empty specification list handling
- ✅ Short-circuit behavior verification

### OR Specification  
- ✅ At least one specification must pass
- ✅ Short-circuit on first success
- ✅ Error aggregation when all fail
- ✅ Nested OR specifications
- ✅ Complex logical combinations

### NOT Specification
- ✅ Logical inversion behavior
- ✅ Error handling (`ErrNotSatisfied`)
- ✅ Nested NOT specifications (double negation)
- ✅ Integration with other specifications
- ✅ Complex logical expressions

## Benchmark Tests

Performance tests covering:
- Individual specification performance
- Filter function scaling (10 to 10,000 items)
- Complex nested specifications
- Memory allocation analysis
- Worst-case scenarios (deep nesting, wide operations)

### Sample Results
```
BenchmarkFilter-4                    7785627    155.1 ns/op    192 B/op    7 allocs/op
BenchmarkAndSpecification-4         61256830     19.97 ns/op      0 B/op    0 allocs/op  
BenchmarkOrSpecification-4           7608762    156.5 ns/op     88 B/op    4 allocs/op
BenchmarkNotSpecification-4       1000000000      1.028 ns/op      0 B/op    0 allocs/op
```

## Best Practices Demonstrated

### Testing Best Practices
- **Comprehensive coverage** - 100% statement coverage
- **Edge case testing** - Nil values, empty collections, error conditions
- **Test organization** - Grouped related tests using test suites
- **Clear naming** - Descriptive test names following Go conventions
- **Arrange/Act/Assert** - Consistent test structure
- **Performance testing** - Benchmarks and regression tests

### Testify Framework Usage
- **Assertions** - Rich assertion methods (`assert.NoError`, `assert.Contains`, etc.)
- **Test suites** - Organized grouping with setup/teardown
- **Requirements** - Critical assertions using `require` 
- **Table-driven tests** - Parameterized test scenarios
- **Panic testing** - Safe panic verification

### Error Testing
- **Error aggregation** - Testing `errors.Join` behavior
- **Error content validation** - Checking error messages
- **Error type checking** - Using `errors.Is` for type verification
- **Nil vs empty** - Distinguishing between nil and empty states

## Running Tests

```bash
# Run all tests
go test ./specification

# Run with verbose output  
go test ./specification -v

# Run specific test suite
go test ./specification -run TestFilterSuite

# Run benchmarks
go test ./specification -bench=.

# Run with race detection
go test ./specification -race

# Generate coverage profile
go test ./specification -coverprofile=coverage.out
```

## Integration Examples

The tests demonstrate real-world usage patterns:

```go
// Complex business logic: Active users 18+ with valid email
complexSpec := NewAndSpecification[TestUser](
    &UserAgeMinSpec{MinAge: 18},
    &UserActiveSpec{},
    &UserEmailValidSpec{},
)

result, err := Filter(users, complexSpec)
```

```go  
// Flexible inclusion: Young OR experienced users
inclusiveSpec := NewOrSpecification[TestUser](
    &UserAgeMaxSpec{MaxAge: 19},  // Young users
    &UserAgeMinSpec{MinAge: 30},  // Experienced users  
)
```

```go
// Exclusion logic: Find inactive users
inactiveSpec := NewNotSpecification[TestUser](&UserActiveSpec{})
```

This comprehensive test suite ensures the specification pattern implementation is robust, performant, and ready for production use.