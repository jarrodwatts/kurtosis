// Code generated by "enumer -type=FlagType -trimprefix=FlagType_ -transform=lower"; DO NOT EDIT.

package flags

import (
	"fmt"
	"strings"
)

const _FlagTypeName = "stringuint32boolstringslice"

var _FlagTypeIndex = [...]uint8{0, 6, 12, 16, 27}

const _FlagTypeLowerName = "stringuint32boolstringslice"

func (i FlagType) String() string {
	if i < 0 || i >= FlagType(len(_FlagTypeIndex)-1) {
		return fmt.Sprintf("FlagType(%d)", i)
	}
	return _FlagTypeName[_FlagTypeIndex[i]:_FlagTypeIndex[i+1]]
}

// An "invalid array index" compiler error signifies that the constant values have changed.
// Re-run the stringer command to generate them again.
func _FlagTypeNoOp() {
	var x [1]struct{}
	_ = x[FlagType_String-(0)]
	_ = x[FlagType_Uint32-(1)]
	_ = x[FlagType_Bool-(2)]
	_ = x[FlagType_StringSlice-(3)]
}

var _FlagTypeValues = []FlagType{FlagType_String, FlagType_Uint32, FlagType_Bool, FlagType_StringSlice}

var _FlagTypeNameToValueMap = map[string]FlagType{
	_FlagTypeName[0:6]:        FlagType_String,
	_FlagTypeLowerName[0:6]:   FlagType_String,
	_FlagTypeName[6:12]:       FlagType_Uint32,
	_FlagTypeLowerName[6:12]:  FlagType_Uint32,
	_FlagTypeName[12:16]:      FlagType_Bool,
	_FlagTypeLowerName[12:16]: FlagType_Bool,
	_FlagTypeName[16:27]:      FlagType_StringSlice,
	_FlagTypeLowerName[16:27]: FlagType_StringSlice,
}

var _FlagTypeNames = []string{
	_FlagTypeName[0:6],
	_FlagTypeName[6:12],
	_FlagTypeName[12:16],
	_FlagTypeName[16:27],
}

// FlagTypeString retrieves an enum value from the enum constants string name.
// Throws an error if the param is not part of the enum.
func FlagTypeString(s string) (FlagType, error) {
	if val, ok := _FlagTypeNameToValueMap[s]; ok {
		return val, nil
	}

	if val, ok := _FlagTypeNameToValueMap[strings.ToLower(s)]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("%s does not belong to FlagType values", s)
}

// FlagTypeValues returns all values of the enum
func FlagTypeValues() []FlagType {
	return _FlagTypeValues
}

// FlagTypeStrings returns a slice of all String values of the enum
func FlagTypeStrings() []string {
	strs := make([]string, len(_FlagTypeNames))
	copy(strs, _FlagTypeNames)
	return strs
}

// IsAFlagType returns "true" if the value is listed in the enum definition. "false" otherwise
func (i FlagType) IsAFlagType() bool {
	for _, v := range _FlagTypeValues {
		if i == v {
			return true
		}
	}
	return false
}
