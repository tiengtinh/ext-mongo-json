package bsonutil

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/mongodb/mongo-tools/common/json"
	"github.com/mongodb/mongo-tools/common/util"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	//	"reflect"
	"time"
)

// ConvertJSONValueToBSON walks through a document or an array and
// replaces any extended JSON value with its corresponding BSON type.
func ConvertJSONValueToBSON(x interface{}) (interface{}, error) {
	switch v := x.(type) {
	case nil:
		return nil, nil
	case bool:
		return v, nil
	case map[string]interface{}: // document
		for key, jsonValue := range v {
			bsonValue, err := ParseJSONValue(jsonValue)
			if err != nil {
				return nil, err
			}
			v[key] = bsonValue
		}
		return v, nil

	case []interface{}: // array
		for i, jsonValue := range v {
			bsonValue, err := ParseJSONValue(jsonValue)
			if err != nil {
				return nil, err
			}
			v[i] = bsonValue
		}
		return v, nil

	case string, float64, int32, int64:
		return v, nil // require no conversion

	case json.ObjectId: // ObjectId
		s := string(v)
		if !bson.IsObjectIdHex(s) {
			return nil, errors.New("expected ObjectId to contain 24 hexadecimal characters")
		}
		return bson.ObjectIdHex(s), nil

	case json.Date: // Date
		n := int64(v)
		return time.Unix(n/1e3, n%1e3*1e6), nil

	case json.ISODate: // ISODate
		n := string(v)
		return util.FormatDate(n)

	case json.NumberLong: // NumberLong
		return int64(v), nil

	case json.NumberInt: // NumberInt
		return int32(v), nil

	case json.NumberFloat: // NumberFloat
		return float64(v), nil
	case json.BinData: // BinData
		data, err := base64.StdEncoding.DecodeString(v.Base64)
		if err != nil {
			return nil, err
		}
		return bson.Binary{v.Type, data}, nil

	case json.DBRef: // DBRef
		return mgo.DBRef{v.Collection, v.Id, v.Database}, nil

	case json.DBPointer: // DBPointer, for backwards compatibility
		return bson.DBPointer{v.Namespace, v.Id}, nil

	case json.RegExp: // RegExp
		return bson.RegEx{v.Pattern, v.Options}, nil

	case json.Timestamp: // Timestamp
		ts := (int64(v.Seconds) << 32) | int64(v.Increment)
		return bson.MongoTimestamp(ts), nil

	case json.JavaScript: // Javascript
		return bson.JavaScript{v.Code, v.Scope}, nil

	case json.MinKey: // MinKey
		return bson.MinKey, nil

	case json.MaxKey: // MaxKey
		return bson.MaxKey, nil

	case json.Undefined: // undefined
		return bson.Undefined, nil

	default:
		return nil, fmt.Errorf("conversion of JSON type '%v' unsupported", v)
	}
}
