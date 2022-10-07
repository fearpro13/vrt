package AMF0

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
)

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
			Type:  "number",
			Value: amf[*pos : *pos+8],
		}
		*pos += 8
		return objectProp, nil
	case 1: // bool 1 byte
		objectProp := &ObjectProperty{
			Type:  "bool",
			Value: amf[*pos : *pos+1],
		}
		*pos++
		return objectProp, nil
	case 2: // string, 16bit length
		stringLength := int(binary.BigEndian.Uint16(amf[*pos : *pos+2]))
		*pos += 2
		objectProp := &ObjectProperty{
			Type:  "string",
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
			Type:  "object",
			Value: buff.Bytes(),
		}

		return objectProp, nil

	case 5: //null
		objectProp := &ObjectProperty{
			Type:  "null",
			Value: nil,
		}
		return objectProp, nil
	default:
		return nil, errors.New(fmt.Sprintf("Unsupported object type %d", objectType))
	}
}

//func Build(object *Object) *[]byte {
//
//}
