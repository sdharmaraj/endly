package endly

import (
	"fmt"
	"github.com/viant/toolbox"
	"reflect"
	"regexp"
	"strings"
)

//Validator represents a validator
type Validator struct {
	ExcludedFields map[string]bool
}

//Check checks expected vs actual value, and returns true if all assertion passes.
func (s *Validator) Check(expected, actual interface{}) (bool, error) {
	var response = &AssertionInfo{}
	err := s.Assert(expected, actual, response, "")
	if err != nil {
		return false, err
	}
	return !response.HasFailure(), nil
}

//Assert check if actual matches expected value, in any case it update assert info with provided validation path.
func (s *Validator) Assert(expected, actual interface{}, assertionInfo *AssertionInfo, path string) error {
	if toolbox.IsValueOfKind(actual, reflect.Slice) {
		if toolbox.IsValueOfKind(expected, reflect.Map) { //convert actual slice to map using expected indexBy directive
			expectedMap := toolbox.AsMap(expected)
			if indexField, ok := expectedMap["@indexBy@"]; ok {
				var actualMap = make(map[string]interface{})
				actualMap["@indexBy@"] = indexField
				var actualSlice = toolbox.AsSlice(actual)
				for _, item := range actualSlice {
					var itemMap = toolbox.AsMap(item)
					if key, has := itemMap[toolbox.AsString(indexField)]; has {
						actualMap[toolbox.AsString(key)] = itemMap
					}
				}
				return s.Assert(expected, actualMap, assertionInfo, path)
			}
		}

		if !toolbox.IsValueOfKind(expected, reflect.Slice) {
			assertionInfo.AddFailure(fmt.Sprintf("Incompatbile types, expected %T but had %v", expected, actual))
			return nil
		}

		err := s.assertSlice(toolbox.AsSlice(expected), toolbox.AsSlice(actual), assertionInfo, path)
		if err != nil {
			return err
		}
		return nil

	}
	if toolbox.IsValueOfKind(actual, reflect.Map) {
		if !toolbox.IsValueOfKind(expected, reflect.Map) {
			assertionInfo.AddFailure(fmt.Sprintf("Incompatbile types, expected %T but had %v", expected, actual))
			return nil
		}
		err := s.assertMap(toolbox.AsMap(expected), toolbox.AsMap(actual), assertionInfo, path)
		if err != nil {
			return err
		}
		return nil
	}
	expectedText := toolbox.AsString(expected)
	actualText := toolbox.AsString(actual)
	s.assertText(expectedText, actualText, assertionInfo, path)
	return nil
}

func (s *Validator) assertText(expected, actual string, response *AssertionInfo, path string) error {
	isRegExpr := strings.HasPrefix(expected, "~/") && strings.HasSuffix(expected, "/")
	isContains := strings.HasPrefix(expected, "/") && strings.HasSuffix(expected, "/")

	if !isRegExpr && !isContains {

		isReversed := strings.HasPrefix(expected, "!")
		if isReversed {
			expected = string(expected[1:])
		}
		if expected != actual && !isReversed {
			response.AddFailure(fmt.Sprintf("[%v]: actual(%T):  '%v' was not equal (%T) '%v'", path, actual, actual, expected, expected))
			return nil
		}
		if expected == actual && isReversed {
			response.AddFailure(fmt.Sprintf("[%v]: actual(%T):  '%v' was not equal (%T) '%v'", path, actual, actual, expected, expected))
			return nil
		}
		response.TestPassed++
		return nil
	}

	if isContains {
		expected = string(expected[1 : len(expected)-1])
		isReversed := strings.HasPrefix(expected, "!")
		if isReversed {
			expected = string(expected[1:])
		}

		if strings.HasPrefix(expected, "[") && strings.HasSuffix(expected, "]") {
			expected = string(expected[1 : len(expected)-1])
			if strings.Contains(expected, "..") {
				var rangeValue = strings.Split(expected, "..")
				var minExpected = toolbox.AsFloat(rangeValue[0])
				var maxExpected = toolbox.AsFloat(rangeValue[1])
				var actualNumber = toolbox.AsFloat(actual)

				if actualNumber >= minExpected && actualNumber <= maxExpected && !isReversed {
					response.TestPassed++
					return nil
				}
				response.AddFailure(fmt.Sprintf("[%v]: actual '%v' is not between'%v and %v'", path, actual, minExpected, maxExpected))

			} else if strings.Contains(expected, ",") {
				var alternatives = strings.Split(expected, ",")
				var doesContain = false
				for _, expectedCandidate := range alternatives {
					if strings.Contains(actual, expectedCandidate) {
						doesContain = true
						break
					}
				}
				if !doesContain && !isReversed {
					response.AddFailure(fmt.Sprintf("[%v]: actual '%v' does not contain: '%v'", path, actual, alternatives))
				} else if isReversed && doesContain {
					response.AddFailure(fmt.Sprintf("[%v]: actual '%v' shold not contain: '%v'", path, actual, alternatives))
				}
				response.TestPassed++
			}
		}
		var doesContain = strings.Contains(actual, expected)
		if !doesContain && !isReversed {
			response.AddFailure(fmt.Sprintf("[%v]: actual '%v' does not contain: '%v'", path, actual, expected))
		} else if isReversed && doesContain {
			response.AddFailure(fmt.Sprintf("[%v]: actual '%v' shold not contain: '%v'", path, actual, expected))
		}
		response.TestPassed++
		return nil
	}

	expected = string(expected[2 : len(expected)-1])
	isReversed := strings.HasPrefix(expected, "!")
	if isReversed {
		expected = string(expected[1:])
	}
	useMultiLine := strings.Index(actual, "\n")
	pattern := ""
	if useMultiLine > 0 {
		pattern = "?m:"
	}
	pattern += expected
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("[%v]: failed to validate '%v' and '%v' due to %v", path, expected, actual, pattern, err)
	}
	var matches = compiled.Match(([]byte)(actual))

	if !matches && !isReversed {
		response.AddFailure(fmt.Sprintf("[%v]: actual: '%v' was not matched %v", path, actual, expected))
	} else if matches && isReversed {
		response.AddFailure(fmt.Sprintf("[%v]: actual: '%v' should not be matched %v", path, actual, expected))
	}
	response.TestPassed++
	return nil
}

func (s *Validator) assertMap(expectedMap map[string]interface{}, actualMap map[string]interface{}, response *AssertionInfo, path string) error {
	for key, expected := range expectedMap {
		if s.ExcludedFields[key] {
			continue
		}
		keyPath := fmt.Sprintf("%v[%v]", path, key)
		actual, ok := actualMap[key]
		if !ok {
			response.AddFailure(fmt.Sprintf("%v was missing", keyPath))
			continue
		}
		if toolbox.AsString(expected) == "@exists@" {
			response.TestPassed++
			continue
		}
		if toolbox.AsString(expected) == "@!exists@" {
			response.AddFailure(fmt.Sprintf("'%v' should not exists but was present: %v", keyPath, actual))
			continue
		}

		err := s.Assert(expected, actual, response, keyPath)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Validator) assertSlice(expectedSlice []interface{}, actualSlice []interface{}, response *AssertionInfo, path string) error {
	for index, expected := range expectedSlice {
		keyPath := fmt.Sprintf("%v[%v]", path, index)
		if !(index < len(actualSlice)) {
			response.AddFailure(fmt.Sprintf("[%v+] were missing, expected size: %v, actual size: %v", keyPath, len(expectedSlice), len(actualSlice)))
			return nil
		}
		actual := actualSlice[index]
		err := s.Assert(expected, actual, response, keyPath)
		if err != nil {
			return err
		}
	}
	return nil
}
