package AMF0

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"math"
)

const StringType = "string"
const NumberType = "number"
const BoolType = "bool"
const NullType = "null"
const ObjectType = "object"
const ECMAArray = "ecma_array"

type ObjectProperty struct {
	Type  string
	Value []byte
}

type Object struct {
	Properties map[string]ObjectProperty
}

type AMF0 struct {
	Objects []ObjectProperty
}

func NewAMF0() *AMF0 {
	amf := &AMF0{Objects: []ObjectProperty{}}
	return amf
}

func NewObject() *Object {
	object := &Object{Properties: map[string]ObjectProperty{}}
	return object
}

func (amf *AMF0) PutNumber(number float64) {
	numBuff := make([]byte, 8)
	binary.BigEndian.PutUint64(numBuff, math.Float64bits(number))
	objectProperty := ObjectProperty{
		Type:  NumberType,
		Value: numBuff,
	}
	amf.Objects = append(amf.Objects, objectProperty)
}

func (amf *AMF0) PutString(str string) {
	objectProperty := ObjectProperty{
		Type:  StringType,
		Value: []byte(str),
	}
	amf.Objects = append(amf.Objects, objectProperty)
}

func (amf *AMF0) PutObject(object *Object) {
	objectProperty := ObjectProperty{
		Type:  ObjectType,
		Value: object.ToBytes(),
	}
	amf.Objects = append(amf.Objects, objectProperty)
}

func (amf *AMF0) PutEcmaArray(object *Object) {
	objectProperty := ObjectProperty{
		Type:  ECMAArray,
		Value: object.ToBytes(),
	}
	amf.Objects = append(amf.Objects, objectProperty)
}

func (object *Object) PutNumber(key string, number float64) {
	numBuff := make([]byte, 8)
	binary.BigEndian.PutUint64(numBuff, math.Float64bits(number))
	objectProperty := ObjectProperty{
		Type:  NumberType,
		Value: numBuff,
	}
	object.Properties[key] = objectProperty
}

func (object *Object) PutString(key string, str string) {
	objectProperty := ObjectProperty{
		Type:  StringType,
		Value: []byte(str),
	}
	object.Properties[key] = objectProperty
}

func (object *Object) PutObject(key string, aObject *Object) {
	objectProperty := ObjectProperty{
		Type:  ObjectType,
		Value: aObject.ToBytes(),
	}
	object.Properties[key] = objectProperty
}

func (object *Object) PutEcmaArray(key string, aObject *Object) {
	objectProperty := ObjectProperty{
		Type:  ECMAArray,
		Value: aObject.ToBytes(),
	}
	object.Properties[key] = objectProperty
}

func Parse(amf0 []byte) (*AMF0, error) {
	amf := &AMF0{Objects: []ObjectProperty{}}

	curPos := 0
	recLimit := 0
	for curPos < len(amf0) {
		if recLimit > len(amf0) {
			return nil, errors.New("recursion limit exceeded")
		}
		objectValue, err := parseValue(amf0, &curPos)
		if err != nil {
			return nil, err
		}

		amf.Objects = append(amf.Objects, *objectValue)
	}

	return amf, nil
}

func parseValue(amf []byte, pos *int) (*ObjectProperty, error) {
	objectType := amf[*pos]
	*pos++
	switch objectType {
	case 0: // num 8 byte
		objectProp := &ObjectProperty{
			Type:  NumberType,
			Value: amf[*pos : *pos+8],
		}
		*pos += 8
		return objectProp, nil
	case 1: // bool 1 byte
		objectProp := &ObjectProperty{
			Type:  BoolType,
			Value: amf[*pos : *pos+1],
		}
		*pos++
		return objectProp, nil
	case 2: // string, 16bit length
		stringLength := int(binary.BigEndian.Uint16(amf[*pos : *pos+2]))
		*pos += 2
		objectProp := &ObjectProperty{
			Type:  StringType,
			Value: amf[*pos : *pos+stringLength],
		}
		*pos += stringLength
		return objectProp, nil
	case 3: // object , 9 - object end, keySize=2 bytes
		object := Object{Properties: map[string]ObjectProperty{}}

		objectEnd := false
		for !objectEnd {
			firstTwoBytes := amf[*pos : *pos+2]
			*pos += 2
			if firstTwoBytes[0] == 0 && firstTwoBytes[1] == 0 && amf[*pos] == 9 {
				*pos++
				objectEnd = true
				break
			}
			objectKeyLen := int(binary.BigEndian.Uint16(firstTwoBytes))
			objectKey := string(amf[*pos : *pos+objectKeyLen])
			*pos += objectKeyLen

			objectValue, err := parseValue(amf, pos)
			if err != nil {
				return nil, err
			}
			object.Properties[objectKey] = *objectValue
		}

		buff := new(bytes.Buffer)
		encoder := gob.NewEncoder(buff)
		encoder.Encode(object.Properties)

		objectProp := &ObjectProperty{
			Type:  ObjectType,
			Value: buff.Bytes(),
		}

		return objectProp, nil

	case 5: //null
		objectProp := &ObjectProperty{
			Type:  NullType,
			Value: nil,
		}
		return objectProp, nil
	case 8: //ecma array
		object := Object{Properties: map[string]ObjectProperty{}}

		//arrayLength := amf[*pos : *pos+4] // IDK WTF it equals 0 when actual objects count > 0
		*pos += 4

		objectEnd := false
		for !objectEnd {
			firstTwoBytes := amf[*pos : *pos+2]
			*pos += 2
			if firstTwoBytes[0] == 0 && firstTwoBytes[1] == 0 && amf[*pos] == 9 {
				*pos++
				objectEnd = true
				break
			}
			objectKeyLen := int(binary.BigEndian.Uint16(firstTwoBytes))
			objectKey := string(amf[*pos : *pos+objectKeyLen])
			*pos += objectKeyLen

			objectValue, err := parseValue(amf, pos)
			if err != nil {
				return nil, err
			}
			object.Properties[objectKey] = *objectValue
		}

		buff := new(bytes.Buffer)
		encoder := gob.NewEncoder(buff)
		encoder.Encode(object.Properties)

		objectProp := &ObjectProperty{
			Type:  ECMAArray,
			Value: buff.Bytes(),
		}

		return objectProp, nil

	default:
		return nil, errors.New(fmt.Sprintf("Unsupported object type %d", objectType))
	}
}

func (amf *AMF0) Build() ([]byte, error) {
	buff := bytes.Buffer{}
	for _, object := range amf.Objects {
		objectBuff, err := build(&object)
		if err != nil {
			return []byte{}, nil
		}
		buff.Write(objectBuff)
	}

	return buff.Bytes(), nil
}

func build(objectProperty *ObjectProperty) ([]byte, error) {
	buff := bytes.Buffer{}
	switch objectProperty.Type {
	case NumberType:
		buff.Write([]byte{0})
		buff.Write(objectProperty.Value)
		return buff.Bytes(), nil
	case StringType:
		buff.Write([]byte{2})
		lenBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBytes, uint16(len(objectProperty.Value)))
		buff.Write(lenBytes)
		buff.Write(objectProperty.Value)
	case BoolType:
		buff.Write([]byte{1})
		buff.Write(objectProperty.Value)
	case NullType:
		buff.Write([]byte{5})
	case ObjectType:
		buff.Write([]byte{3})
		var objectMap map[string]ObjectProperty
		objectBuf := bytes.NewBuffer(objectProperty.Value)
		decoder := gob.NewDecoder(objectBuf)
		err := decoder.Decode(&objectMap)
		if err != nil {
			return []byte{}, nil
		}
		for objectMapKey, objectMapEntry := range objectMap {
			objectMapEntryBuf, err := build(&objectMapEntry)
			if err != nil {
				return []byte{}, nil
			}
			lenBytes := make([]byte, 2)
			binary.BigEndian.PutUint16(lenBytes, uint16(len(objectMapKey)))
			buff.Write(lenBytes)
			buff.WriteString(objectMapKey)
			buff.Write(objectMapEntryBuf)
		}
		buff.Write([]byte{0, 0, 9})
	case ECMAArray:
		buff.Write([]byte{8})
		var objectMap map[string]ObjectProperty
		objectBuf := bytes.NewBuffer(objectProperty.Value)
		decoder := gob.NewDecoder(objectBuf)
		err := decoder.Decode(&objectMap)
		if err != nil {
			return []byte{}, nil
		}

		arrayLenBytes := make([]byte, 4)
		//binary.BigEndian.PutUint32(arrayLenBytes, uint32(len(objectMap)))
		binary.BigEndian.PutUint32(arrayLenBytes, 0)
		buff.Write(arrayLenBytes)

		for objectMapKey, objectMapEntry := range objectMap {
			objectMapEntryBuf, err := build(&objectMapEntry)
			if err != nil {
				return []byte{}, nil
			}
			lenBytes := make([]byte, 2)
			binary.BigEndian.PutUint16(lenBytes, uint16(len(objectMapKey)))
			buff.Write(lenBytes)
			buff.WriteString(objectMapKey)
			buff.Write(objectMapEntryBuf)
		}
		buff.Write([]byte{0, 0, 9})
	default:
		return []byte{}, errors.New(fmt.Sprintf("Unsupported property type %s", objectProperty.Type))
	}

	return buff.Bytes(), nil
}

func (object *Object) ToBytes() []byte {
	buff := new(bytes.Buffer)
	encoder := gob.NewEncoder(buff)
	encoder.Encode(object.Properties)
	return buff.Bytes()
}
