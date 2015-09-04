/*
  Copyright (c) 2015 StarBrilliant <m13253@hotmail.com>
  All rights reserved.
 
  Redistribution and use in source and binary forms are permitted
  provided that the above copyright notice and this paragraph are
  duplicated in all such forms and that any documentation,
  advertising materials, and other materials related to such
  distribution and use acknowledge that the software was developed by
  StarBrilliant.
  The name of StarBrilliant may not be used to endorse or promote
  products derived from this software without specific prior written
  permission.
 
  THIS SOFTWARE IS PROVIDED ``AS IS'' AND WITHOUT ANY EXPRESS OR
  IMPLIED WARRANTIES, INCLUDING, WITHOUT LIMITATION, THE IMPLIED
  WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE.
*/

package jksn

import (
    "bytes"
    "container/list"
    "encoding/binary"
    "io"
    "math"
    "math/big"
    "reflect"
    "strconv"
)

type UnsupportedTypeError struct {
    Type    reflect.Type
}

func (self *UnsupportedTypeError) Error() string {
    return "jksn: unsupported type: " + self.Type.String()
}

type UnsupportedValueError struct {
    Value   reflect.Value
    Str     string
}

func (self *UnsupportedValueError) Error() string {
    return "jksn: unsupported value: " + self.Str
}

type UnmarshalTypeError struct {
    Value   string
    Type    reflect.Type
    Offset  int64
}

func (self *UnmarshalTypeError) Error() string {
    return "jksn: cannot unmarshal " + self.Value + " into Go value of type " + self.Type.String()
}

type UnmarshalFieldError struct {
    Key     string
    Type    reflect.Type
    Field   reflect.StructField
}

func (self *UnmarshalFieldError) Error() string {
    return "jksn: cannot unmarshal object key " + strconv.Quote(self.Key) + " into unexported field " + self.Field.Name + " of type " + self.Type.String()
}

type InvalidUnmarshalError struct {
    Type reflect.Type
}

func (self *InvalidUnmarshalError) Error() string {
    if self.Type == nil {
        return "jksn: Unmarshal(nil)"
    } else if self.Type.Kind() != reflect.Ptr {
        return "jksn: Unmarshal(non-pointer " + self.Type.String() + ")"
    } else {
        return "jksn: Unmarshal(nil " + self.Type.String() + ")"
    }
}

func Marshal(obj interface{}) (res []byte, err error) {
    var buf bytes.Buffer
    err = NewEncoder(&buf).Encode(obj)
    res = buf.Bytes()
    return
}

type jksn_proxy struct {
    Origin      interface{}
    Control     uint8
    Data        []byte
    Buf         []byte
    Children    *list.List
    Hash        *uint8
}

func new_jksn_proxy(origin interface{}, control uint8, data []byte, buf []byte) (res *jksn_proxy) {
    res = new(jksn_proxy)
    res.Origin = origin
    res.Control = control
    res.Data = make([]byte, len(data))
    copy(res.Data, data)
    res.Buf = make([]byte, len(buf))
    copy(res.Buf, buf)
    res.Children = list.New()
    return
}

func (self *jksn_proxy) Output(fp io.Writer, recursive bool) (err error) {
    control := [1]byte{ self.Control }
    _, err = fp.Write(control[:])
    if err != nil { return }
    _, err = fp.Write(self.Data)
    if err != nil { return }
    _, err = fp.Write(self.Buf)
    if err != nil { return }
    if recursive {
        for i := self.Children.Front(); i != nil; i = i.Next() {
            err = i.Value.(*jksn_proxy).Output(fp, true)
            if err != nil { return }
        }
    }
    return
}

func (self *jksn_proxy) Len(depth uint) (result int64) {
    result = 1 + int64(len(self.Data)) + int64(len(self.Buf))
    if depth == 0 {
        for i := self.Children.Front(); i != nil; i = i.Next() {
            result += i.Value.(*jksn_proxy).Len(0);
        }
    } else if depth != 1 {
        for i := self.Children.Front(); i != nil; i = i.Next() {
            result += i.Value.(*jksn_proxy).Len(depth-1);
        }
    }
    return
}

var empty_byte_array = [0]byte{}
var empty_bytes = empty_byte_array[:]

type unspecified struct {}

var unspecified_value = unspecified{}

type Encoder struct {
    writer io.Writer
    firsterr error
    lastint big.Int
    texthash [256][]byte
    blobhash [256][]byte
}

func NewEncoder(writer io.Writer) (res *Encoder) {
    res = new(Encoder)
    res.writer = writer
    return
}

func (self *Encoder) Encode(obj interface{}) (err error) {
    self.firsterr = nil
    result := self.dump_to_proxy(obj)
    _, err = self.writer.Write([]byte("jk!"))
    if err == nil {
        err = result.Output(self.writer, true)
    }
    if self.firsterr != nil {
        return self.firsterr
    }
    return
}

func (self *Encoder) dump_to_proxy(obj interface{}) *jksn_proxy {
    return self.dump_value(obj)
}

func (self *Encoder) dump_value(obj interface{}) *jksn_proxy {
    if obj == nil {
        return self.dump_nil(nil)
    } else {
        value := reflect.ValueOf(obj)
        for value.Kind() == reflect.Ptr {
            if value.IsNil() {
                return self.dump_nil(nil)
            } else {
                value = reflect.Indirect(value)
                obj = value.Interface()
            }
        }
        switch value.Kind() {
            case reflect.Bool:
                return self.dump_bool(value.Bool())
            case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
                return self.dump_int(big.NewInt(value.Int()))
            case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
                return self.dump_int(big.NewInt(int64(value.Uint())))
            case reflect.Uint64: {
                value_uint64 := value.Uint()
                value_big := big.NewInt(int64(value_uint64 >> 1))
                value_big.Lsh(value_big, 1)
                value_big.Or(value_big, big.NewInt(int64(value_uint64 & 0x1)))
                return self.dump_int(value_big)
            }
            case reflect.Float32:
                return self.dump_float32(obj.(float32))
            case reflect.Float64:
                return self.dump_float64(obj.(float64))
            default:
                self.firsterr = &UnsupportedTypeError{ value.Type() }
                return self.dump_nil(nil)
        }
    }
}

func (self *Encoder) dump_nil(obj interface{}) *jksn_proxy {
    return new_jksn_proxy(obj, 0x01, empty_bytes, empty_bytes)
}

func (self *Encoder) dump_unspecified(obj *unspecified) *jksn_proxy {
    return new_jksn_proxy(obj, 0xa0, empty_bytes, empty_bytes)
}

func (self *Encoder) dump_bool(obj bool) *jksn_proxy {
    if obj {
        return new_jksn_proxy(obj, 0x03, empty_bytes, empty_bytes)
    } else {
        return new_jksn_proxy(obj, 0x02, empty_bytes, empty_bytes)
    }
}

func (self *Encoder) dump_int(obj *big.Int) *jksn_proxy {
    if obj.Sign() >= 0 && obj.Cmp(big.NewInt(0xa)) <= 0 {
        return new_jksn_proxy(obj, 0x10 | uint8(obj.Uint64()), empty_bytes, empty_bytes)
    } else if obj.Cmp(big.NewInt(-0x80)) >= 0 && obj.Cmp(big.NewInt(0x7f)) <= 0 {
        return new_jksn_proxy(obj, 0x1d, self.encode_int(obj, 1), empty_bytes)
    } else if obj.Cmp(big.NewInt(-0x8000)) >= 0 && obj.Cmp(big.NewInt(0x7fff)) <= 0 {
        return new_jksn_proxy(obj, 0x1c, self.encode_int(obj, 2), empty_bytes)
    } else if (
        (obj.Cmp(big.NewInt(-0x80000000)) >= 0 && obj.Cmp(big.NewInt(-0x200000)) <= 0) &&
        (obj.Cmp(big.NewInt(0x200000)) >= 0 && obj.Cmp(big.NewInt(0x7fffffff)) <= 0)) {
        return new_jksn_proxy(obj, 0x1b, self.encode_int(obj, 4), empty_bytes)
    } else if obj.Sign() >= 0 {
        return new_jksn_proxy(obj, 0x1f, self.encode_int(obj, 0), empty_bytes)
    } else {
        return new_jksn_proxy(obj, 0x1e, self.encode_int(new(big.Int).Neg(obj), 0), empty_bytes)
    }
}

func (self *Encoder) dump_float32(obj float32) *jksn_proxy {
    obj_float64 := float64(obj)
    if math.IsNaN(obj_float64) {
        return new_jksn_proxy(obj, 0x20, empty_bytes, empty_bytes)
    } else if math.IsInf(obj_float64, 1) {
        return new_jksn_proxy(obj, 0x2f, empty_bytes, empty_bytes)
    } else if math.IsInf(obj_float64, -1) {
        return new_jksn_proxy(obj, 0x2e, empty_bytes, empty_bytes)
    } else {
        var buf bytes.Buffer
        binary.Write(&buf, binary.BigEndian, obj)
        if buf.Len() != 4 {
            panic("jksn: buf.Len() != 4")
        }
        return new_jksn_proxy(obj, 0x2d, buf.Bytes(), empty_bytes)
    }
}

func (self *Encoder) dump_float64(obj float64) *jksn_proxy {
    if math.IsNaN(obj) {
        return new_jksn_proxy(obj, 0x20, empty_bytes, empty_bytes)
    } else if math.IsInf(obj, 1) {
        return new_jksn_proxy(obj, 0x2f, empty_bytes, empty_bytes)
    } else if math.IsInf(obj, -1) {
        return new_jksn_proxy(obj, 0x2e, empty_bytes, empty_bytes)
    } else {
        var buf bytes.Buffer
        binary.Write(&buf, binary.BigEndian, obj)
        if buf.Len() != 8 {
            panic("jksn: buf.Len() != 8")
        }
        return new_jksn_proxy(obj, 0x2c, buf.Bytes(), empty_bytes)
    }
}

func (self *Encoder) encode_int(number *big.Int, size uint) []byte {
    if size == 1 {
        return []byte{ uint8(int8(number.Int64())) }
    } else if size == 2 {
        number_buf := uint16(int16(number.Int64()))
        return []byte{
            uint8(number_buf >> 8),
            uint8(number_buf),
        }
    } else if size == 4 {
        number_buf := uint32(int32(number.Int64()))
        return []byte{
            uint8(number_buf >> 24),
            uint8(number_buf >> 16),
            uint8(number_buf >> 8),
            uint8(number_buf),
        }
    } else if size == 0 {
        if number.Sign() < 0 {
            panic("jksn: number < 0")
        }
        result := []byte{ uint8(new(big.Int).And(number, big.NewInt(0x7f)).Uint64()) }
        number.Rsh(number, 7)
        for number.Sign() != 0 {
            result = append(result, uint8(new(big.Int).And(number, big.NewInt(0x7f)).Uint64()) | 0x80)
            number.Rsh(number, 7)
        }
        for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
            result[i], result[j] = result[j], result[i]
        }
        return result
    } else {
        panic("jksn: size not in (1, 2, 4, 0)")
    }
}
