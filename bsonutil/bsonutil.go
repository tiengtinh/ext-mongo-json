// Package bsonutil provides utilities for processing BSON data.
package bsonutil

import (
	//	"encoding/base64"
	//	"encoding/hex"
	"errors"
	"fmt"
	"github.com/mongodb/mongo-tools/common/json"
	"github.com/mongodb/mongo-tools/common/util"
	//	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"strconv"
	"time"
)

var ErrNoSuchField = errors.New("no such field")

// ConvertJSONDocumentToBSON iterates through the document map and converts JSON
// values to their corresponding BSON values. It also replaces any extended JSON
// type value (e.g. $date) with the corresponding BSON type.
func ConvertJSONDocumentToBSON(doc map[string]interface{}) error {
	for key, jsonValue := range doc {
		var bsonValue interface{}
		var err error

		switch v := jsonValue.(type) {
		case map[string]interface{}: // subdocument
			bsonValue, err = ParseSpecialKeys(v)

		default:
			bsonValue, err = ConvertJSONValueToBSON(v)
		}
		if err != nil {
			return err
		}

		doc[key] = bsonValue
	}
	return nil
}

// ParseSpecialKeys takes a JSON document and inspects it for any extended JSON
// type (e.g $numberLong) and replaces any such values with the corresponding
// BSON type.
func ParseSpecialKeys(doc map[string]interface{}) (interface{}, error) {
	switch len(doc) {
	case 1: // document has a single field
		if jsonValue, ok := doc["$date"]; ok {
			switch v := jsonValue.(type) {
			case string:
				return util.FormatDate(v)
			case map[string]interface{}:
				if jsonValue, ok := v["$numberLong"]; ok {
					n, err := parseNumberLongField(jsonValue)
					if err != nil {
						return nil, err
					}
					return time.Unix(n/1e3, n%1e3*1e6), err
				}
				return nil, errors.New("expected $numberLong field in $date")

			case json.Number:
				n, err := v.Int64()
				return time.Unix(n/1e3, n%1e3*1e6), err

			case float64:
				n := int64(v)
				return time.Unix(n/1e3, n%1e3*1e6), nil
			case int64:
				return time.Unix(v/1e3, v%1e3*1e6), nil

			case json.ISODate:
				return v, nil

			default:
				return nil, errors.New("invalid type for $date field")
			}
		}

		const relativeDateFormat string = "now-\\d+y"
		if jsonValue, ok := doc["$relative_date"]; ok {
			switch v := jsonValue.(type) {
			case string:

				return util.FormatDate(v)
			default:
				return nil, errors.New("invalid type for $relative_date field")
			}
		}

		if jsonValue, ok := doc["$code"]; ok {
			switch v := jsonValue.(type) {
			case string:
				return bson.JavaScript{Code: v}, nil
			default:
				return nil, errors.New("expected $code field to have string value")
			}
		}

		if jsonValue, ok := doc["$oid"]; ok {
			switch v := jsonValue.(type) {
			case string:
				if !bson.IsObjectIdHex(v) {
					return nil, errors.New("expected $oid field to contain 24 hexadecimal character")
				}
				return bson.ObjectIdHex(v), nil

			default:
				return nil, errors.New("expected $oid field to have string value")
			}
		}

		if jsonValue, ok := doc["$numberLong"]; ok {
			return parseNumberLongField(jsonValue)
		}

		if jsonValue, ok := doc["$numberInt"]; ok {
			switch v := jsonValue.(type) {
			case string:
				// all of decimal, hex, and octal are supported here
				n, err := strconv.ParseInt(v, 0, 32)
				return int32(n), err

			default:
				return nil, errors.New("expected $numberInt field to have string value")
			}
		}

		if jsonValue, ok := doc["$timestamp"]; ok {
			ts := json.Timestamp{}

			tsDoc, ok := jsonValue.(map[string]interface{})
			if !ok {
				return nil, errors.New("expected $timestamp key to have internal document")
			}

			if seconds, ok := tsDoc["t"]; ok {
				if asUint32, err := util.ToUInt32(seconds); err == nil {
					ts.Seconds = asUint32
				} else {
					return nil, errors.New("expected $timestamp 't' field to be a numeric type")
				}
			} else {
				return nil, errors.New("expected $timestamp to have 't' field")
			}
			if inc, ok := tsDoc["i"]; ok {
				if asUint32, err := util.ToUInt32(inc); err == nil {
					ts.Increment = asUint32
				} else {
					return nil, errors.New("expected $timestamp 'i' field to be  a numeric type")
				}
			} else {
				return nil, errors.New("expected $timestamp to have 'i' field")
			}
			// see BSON spec for details on the bit fiddling here
			return bson.MongoTimestamp(int64(ts.Seconds)<<32 | int64(ts.Increment)), nil
		}

		if _, ok := doc["$undefined"]; ok {
			return bson.Undefined, nil
		}

	case 2: // document has two fields
		if jsonValue, ok := doc["$code"]; ok {
			code := bson.JavaScript{}
			switch v := jsonValue.(type) {
			case string:
				code.Code = v
			default:
				return nil, errors.New("expected $code field to have string value")
			}

			if jsonValue, ok = doc["$scope"]; ok {
				switch v2 := jsonValue.(type) {
				case map[string]interface{}:
					x, err := ParseSpecialKeys(v2)
					if err != nil {
						return nil, err
					}
					code.Scope = x
					return code, nil
				default:
					return nil, errors.New("expected $scope field to contain map")
				}
			} else {
				return nil, errors.New("expected $scope field with $code field")
			}
		}

		if jsonValue, ok := doc["$regex"]; ok {
			regex := bson.RegEx{}

			switch pattern := jsonValue.(type) {
			case string:
				regex.Pattern = pattern

			default:
				return nil, errors.New("expected $regex field to have string value")
			}
			if jsonValue, ok = doc["$options"]; !ok {
				return nil, errors.New("expected $options field with $regex field")
			}

			switch options := jsonValue.(type) {
			case string:
				regex.Options = options

			default:
				return nil, errors.New("expected $options field to have string value")
			}

			// Validate regular expression options
			for i := range regex.Options {
				switch o := regex.Options[i]; o {
				default:
					return nil, fmt.Errorf("invalid regular expression option '%v'", o)

				case 'g', 'i', 'm', 's': // allowed
				}
			}
			return regex, nil
		}
	}

	// Did not match any special ('$') keys, so convert all sub-values.
	return ConvertJSONValueToBSON(doc)
}

func parseNumberLongField(jsonValue interface{}) (int64, error) {
	switch v := jsonValue.(type) {
	case string:
		// all of decimal, hex, and octal are supported here
		return strconv.ParseInt(v, 0, 64)

	default:
		return 0, errors.New("expected $numberLong field to have string value")
	}
}

// ParseJSONValue takes any value generated by the json package and returns a
// BSON version of that value.
func ParseJSONValue(jsonValue interface{}) (interface{}, error) {
	switch v := jsonValue.(type) {
	case map[string]interface{}: // subdocument
		return ParseSpecialKeys(v)

	default:
		return ConvertJSONValueToBSON(v)
	}
}
