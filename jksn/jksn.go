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
    "bufio"
    "bytes"
    "encoding/binary"
    "encoding/json"
    "fmt"
    "io"
    "math"
    "math/big"
    "reflect"
    "strconv"
    "unicode/utf16"
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

type SyntaxError struct {
    msg     string
    Offset  int64
}

func (self *SyntaxError) Error() string {
    return self.msg
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
    Type    reflect.Type
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
    buf := new(bytes.Buffer)
    err = NewEncoder(buf).Encode(obj)
    res = buf.Bytes()
    return
}

func Unmarshal(data []byte, obj interface{}) (err error) {
    buf := bytes.NewBuffer(data)
    err = NewDecoder(buf).Decode(obj)
    return
}

type jksn_proxy struct {
    Origin      interface{}
    Control     uint8
    Data        []byte
    Buf         []byte
    Children    []*jksn_proxy
    Hash        uint8
}

func new_jksn_proxy(origin interface{}, control uint8, data []byte, buf []byte) (res *jksn_proxy) {
    res = new(jksn_proxy)
    res.Origin = origin
    res.Control = control
    res.Data = make([]byte, len(data))
    copy(res.Data, data)
    res.Buf = make([]byte, len(buf))
    copy(res.Buf, buf)
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
        for _, i := range self.Children {
            err = i.Output(fp, true)
            if err != nil { return }
        }
    }
    return
}

func (self *jksn_proxy) Len(depth uint) (result int64) {
    result = 1 + int64(len(self.Data)) + int64(len(self.Buf))
    if depth == 0 {
        for _, i := range self.Children {
            result += i.Len(0);
        }
    } else if depth != 1 {
        for _, i := range self.Children {
            result += i.Len(depth-1);
        }
    }
    return
}

var empty_byte_array = [0]byte{}
var empty_bytes = empty_byte_array[:]

type unspecified struct {}

var unspecified_value = unspecified{}

type Encoder struct {
    writer      io.Writer
    firsterr    error
    lastint     *big.Int
    texthash    [256][]byte
    blobhash    [256][]byte
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
    return self.optimize(self.dump_value(obj))
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
            return self.dump_bool(obj.(bool))
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
        case reflect.String:
            return self.dump_string(obj.(string))
        case reflect.Array, reflect.Slice:
            switch value.Type().Kind() {
            case reflect.Uint8:
                return self.dump_bytes(obj.([]byte))
            default:
                obj_array := make([]interface{}, value.Len())
                for i := 0; i < value.Len(); i++ {
                    obj_array[i] = value.Index(i).Interface()
                }
                return self.dump_slice(obj_array)
            }
        case reflect.Map: {
            obj_keys := value.MapKeys()
            obj_map := make(map[interface{}]interface{}, len(obj_keys))
            for _, key := range obj_keys {
                obj_map[key.Interface()] = value.MapIndex(key).Interface()
            }
            return self.dump_map(obj_map)
        }
        case reflect.Struct:
            switch obj.(type) {
            case unspecified:
                return self.dump_unspecified(obj.(unspecified))
            case big.Int: {
                obj_bigint := obj.(big.Int)
                return self.dump_int(&obj_bigint)
            }
            default:
                return self.dump_map(self.struct_to_map(obj))
            }
        default:
            self.store_err(&UnsupportedTypeError{ value.Type() })
            return self.dump_nil(nil)
        }
    }
}

func (self *Encoder) dump_nil(obj interface{}) *jksn_proxy {
    return new_jksn_proxy(obj, 0x01, empty_bytes, empty_bytes)
}

func (self *Encoder) dump_unspecified(obj unspecified) *jksn_proxy {
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

func (self *Encoder) dump_string(obj string) (result *jksn_proxy) {
    obj_utf16 := utf8_to_utf16le(obj)
    obj_short, control, length := []byte(obj), uint8(0x40), len(obj)
    is_utf16 := false
    if len(obj_utf16) < len(obj) {
        obj_short, control, length = obj_utf16, 0x30, len(obj_utf16)/2
        is_utf16 = true
    }
    if length <= 0xb {
        result = new_jksn_proxy(obj, control | uint8(length), empty_bytes, obj_short)
    } else if !is_utf16 && length == 0xc {
        result = new_jksn_proxy(obj, control | uint8(length), empty_bytes, obj_short)
    } else if length <= 0xff {
        result = new_jksn_proxy(obj, control | 0xe, self.encode_int(big.NewInt(int64(length)), 1), obj_short)
    } else if length <= 0xffff {
        result = new_jksn_proxy(obj, control | 0xd, self.encode_int(big.NewInt(int64(length)), 2), obj_short)
    } else {
        result = new_jksn_proxy(obj, control | 0xf, self.encode_int(big.NewInt(int64(length)), 0), obj_short)
    }
    result.Hash = djb_hash(obj_short)
    return
}

func (self *Encoder) dump_bytes(obj []byte) (result *jksn_proxy) {
    length := len(obj)
    if length <= 0xb {
        result = new_jksn_proxy(obj, 0x50 | uint8(length), empty_bytes, obj)
    } else if length <= 0xff {
        result = new_jksn_proxy(obj, 0x5e, self.encode_int(big.NewInt(int64(length)), 1), obj)
    } else if length <= 0xffff {
        result = new_jksn_proxy(obj, 0x5d, self.encode_int(big.NewInt(int64(length)), 2), obj)
    } else {
        result = new_jksn_proxy(obj, 0x5f, self.encode_int(big.NewInt(int64(length)), 0), obj)
    }
    result.Hash = djb_hash(obj)
    return
}

func (self *Encoder) dump_slice(obj []interface{}) (result *jksn_proxy) {
    result = self.encode_straight_slice(obj)
    if ok, as_map := self.test_swap_availability(obj); ok {
        result_swapped := self.encode_swapped_slice(as_map)
        if result_swapped.Len(3) < result.Len(3) {
            result = result_swapped
        }
    }
    return
}

func (self *Encoder) test_swap_availability(obj []interface{}) (columns bool, as_map []map[interface{}]interface{}) {
    as_map = make([]map[interface{}]interface{}, len(obj))
    for i, row := range obj {
        value := reflect.ValueOf(row)
        for value.Kind() == reflect.Ptr {
            if value.IsNil() {
                return false, nil
            } else {
                value = reflect.Indirect(value)
                row = value.Interface()
            }
        }
        switch value.Kind() {
        case reflect.Map:
            row_keys := value.MapKeys()
            as_map[i] = make(map[interface{}]interface{}, len(row_keys))
            for _, key := range row_keys {
                as_map[i][key.Interface()] = value.MapIndex(key).Interface()
            }
            if value.Len() != 0 {
                columns = true
            }
        case reflect.Struct:
            switch row.(type) {
            case big.Int:
                return false, nil
            default:
                as_map[i] = self.struct_to_map(row)
                if len(as_map[i]) != 0 {
                    columns = true
                }
            }
        default:
            return false, nil
        }
    }
    return
}

func (self *Encoder) encode_straight_slice(obj []interface{}) (result *jksn_proxy) {
    length := len(obj)
    if length <= 0xc {
        result = new_jksn_proxy(obj, 0x80 | uint8(length), empty_bytes, empty_bytes)
    } else if length <= 0xff {
        result = new_jksn_proxy(obj, 0x8e, self.encode_int(big.NewInt(int64(length)), 1), empty_bytes)
    } else if length <= 0xffff {
        result = new_jksn_proxy(obj, 0x8d, self.encode_int(big.NewInt(int64(length)), 2), empty_bytes)
    } else {
        result = new_jksn_proxy(obj, 0x8f, self.encode_int(big.NewInt(int64(length)), 0), empty_bytes)
    }
    result.Children = make([]*jksn_proxy, length)
    for i := 0; i < length; i++ {
        result.Children[i] = self.dump_value(obj[i])
    }
    return
}

func (self *Encoder) encode_swapped_slice(obj []map[interface{}]interface{}) (result *jksn_proxy) {
    columns := make(map[interface{}]bool)
    for _, row := range obj {
        for column := range row {
            columns[column] = true
        }
    }
    collen := len(columns)
    if collen <= 0xc {
        result = new_jksn_proxy(obj, 0xa0 | uint8(collen), empty_bytes, empty_bytes)
    } else if collen <= 0xff {
        result = new_jksn_proxy(obj, 0xae, self.encode_int(big.NewInt(int64(collen)), 1), empty_bytes)
    } else if collen <= 0xffff {
        result = new_jksn_proxy(obj, 0xad, self.encode_int(big.NewInt(int64(collen)), 2), empty_bytes)
    } else {
        result = new_jksn_proxy(obj, 0xaf, self.encode_int(big.NewInt(int64(collen)), 0), empty_bytes)
    }
    result.Children = make([]*jksn_proxy, 0, collen*2)
    for column := range columns {
        result.Children = append(result.Children, self.dump_value(column))
        columns_value := make([]interface{}, len(obj))
        for i, row := range obj {
            if item, ok := row[column]; ok {
                columns_value[i] = item
            } else {
                columns_value[i] = unspecified_value
            }
        }
        result.Children = append(result.Children, self.dump_slice(columns_value))
    }
    return
}

func (self *Encoder) dump_map(obj map[interface{}]interface{}) (result *jksn_proxy) {
    length := len(obj)
    if length <= 0xc {
        result = new_jksn_proxy(obj, 0x90 | uint8(length), empty_bytes, empty_bytes)
    } else if length <= 0xff {
        result = new_jksn_proxy(obj, 0x9e, self.encode_int(big.NewInt(int64(length)), 1), empty_bytes)
    } else if length <= 0xffff {
        result = new_jksn_proxy(obj, 0x9d, self.encode_int(big.NewInt(int64(length)), 2), empty_bytes)
    } else {
        result = new_jksn_proxy(obj, 0x9f, self.encode_int(big.NewInt(int64(length)), 0), empty_bytes)
    }
    result.Children = make([]*jksn_proxy, 0, length*2)
    for key, value := range obj {
        result.Children = append(result.Children, self.dump_value(key), self.dump_value(value))
    }
    if len(result.Children) != length*2 {
        panic("jksn: len(result.Children) != length*2")
    }
    return result
}

func (self *Encoder) struct_to_map(obj interface{}) (result map[interface{}]interface{}) {
    obj_value := reflect.ValueOf(obj)
    obj_type := obj_value.Type()
    result = make(map[interface{}]interface{})
    for field := 0; field < obj_type.NumField(); field++ {
        field_type := obj_type.Field(field)
        tag_name := field_type.Tag.Get("jksn")
        if len(tag_name) == 0 {
            tag_name = field_type.Tag.Get("json")
            if len(tag_name) == 0 {
                tag_name = field_type.Name
            }
        }
        if len(tag_name) != 0 && tag_name != "-" {
            result[tag_name] = obj_value.Field(field).Interface()
        }
    }
    return
}

func (self *Encoder) optimize(obj *jksn_proxy) *jksn_proxy {
    control := obj.Control & 0xf0
    if control == 0x10 {
        if self.lastint != nil {
            origin_int := obj.Origin.(*big.Int)
            delta := new(big.Int).Sub(origin_int, self.lastint)
            if new(big.Int).Abs(delta).Cmp(new(big.Int).Abs(origin_int)) < 0 {
                var new_control uint8
                var new_data []byte
                if delta.Sign() >= 0 && delta.Cmp(big.NewInt(0x5)) <= 0 {
                    new_control, new_data = 0xd0 | uint8(delta.Uint64()), empty_bytes
                } else if delta.Cmp(big.NewInt(-0x5)) >= 0 && delta.Cmp(big.NewInt(-0x1)) <= 0 {
                    new_control, new_data = 0xd0 | uint8(new(big.Int).Add(delta, big.NewInt(11)).Uint64()), empty_bytes
                } else if delta.Cmp(big.NewInt(-0x80)) >= 0 && delta.Cmp(big.NewInt(0x7f)) <= 0 {
                    new_control, new_data = 0xdd, self.encode_int(delta, 1)
                } else if delta.Cmp(big.NewInt(-0x8000)) >= 0 && delta.Cmp(big.NewInt(0x7fff)) <= 0 {
                    new_control, new_data = 0xdc, self.encode_int(delta, 2)
                } else if (
                    (delta.Cmp(big.NewInt(-0x80000000)) >= 0 && delta.Cmp(big.NewInt(-0x200000)) <= 0) ||
                    (delta.Cmp(big.NewInt(0x200000)) >= 0 && delta.Cmp(big.NewInt(0x7fffffff)) <= 0)) {
                    new_control, new_data = 0xdb, self.encode_int(delta, 4)
                } else if delta.Sign() >= 0 {
                    new_control, new_data = 0xdf, self.encode_int(delta, 0)
                } else {
                    new_control, new_data = 0xde, self.encode_int(new(big.Int).Neg(delta), 0)
                }
                if len(new_data) < len(obj.Data) {
                    obj.Control, obj.Data = new_control, new_data
                }
            }
        }
        self.lastint = obj.Origin.(*big.Int)
    } else if control == 0x30 || control == 0x40 {
        if len(obj.Buf) > 1 {
            if bytes.Equal(self.texthash[obj.Hash], obj.Buf) {
                obj.Control, obj.Data, obj.Buf = 0x3c, []byte{ obj.Hash }, empty_bytes
            } else {
                self.texthash[obj.Hash] = obj.Buf
            }
        }
    } else if control == 0x50 {
        if len(obj.Buf) > 1 {
            if bytes.Equal(self.blobhash[obj.Hash], obj.Buf) {
                obj.Control, obj.Data, obj.Buf = 0x5c, []byte{ obj.Hash }, empty_bytes
            } else {
                self.blobhash[obj.Hash] = make([]byte, len(obj.Buf))
                copy(self.blobhash[obj.Hash], obj.Buf)
            }
        }
    } else {
        for _, child := range obj.Children {
            self.optimize(child)
        }
    }
    return obj
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

func (self *Encoder) store_err(err error) error {
    if self.firsterr == nil {
        self.firsterr = err
    }
    return self.firsterr
}

type Decoder struct {
    reader      *bufio.Reader
    readcount   int64
    firsterr    error
    lastint     *big.Int
    texthash    [256]*string
    blobhash    [256][]byte
}

func NewDecoder(reader io.Reader) (res *Decoder) {
    res = new(Decoder)
    if buf_reader, ok := reader.(*bufio.Reader); ok {
        res.reader = buf_reader
    } else {
        res.reader = bufio.NewReader(reader)
    }
    return
}

func (self *Decoder) Buffered() io.Reader {
    return self.reader
}

func (self *Decoder) Decode(obj interface{}) (err error) {
    self.readcount = 0
    header, header_err := self.reader.Peek(3)
    if header_err == nil && bytes.Equal(header, []byte("jk!")) {
        discarded, _ := self.reader.Discard(len(header))
        if discarded != len(header) {
            panic("jksn: discarded != len(header)")
        }
        self.readcount += int64(discarded)
    }
    generic_value := self.load_value()
    self.fit_type(obj, generic_value)
    return self.firsterr
}

func (self *Decoder) load_value() interface{} {
    for {
        control, err := self.reader.ReadByte()
        self.store_err(err)
        if err != nil {
            return nil
        }
        self.readcount++
        ctrlhi := control & 0xf0
        switch ctrlhi {
        // Special values
        case 0x00:
            switch control {
            case 0x00, 0x01:
                return nil
            case 0x02:
                return false
            case 0x03:
                return true
            case 0x0f: {
                json_literal := self.load_value()
                if s, ok := json_literal.(string); ok {
                    var result interface{}
                    self.store_err(json.Unmarshal([]byte(s), &result))
                    return result
                } else {
                    self.store_err(&SyntaxError{
                        "jksn: JKSN value 0x0f requires a string but found: " + reflect.TypeOf(json_literal).String(),
                        self.readcount,
                    })
                }
            }
            }
        // Integers
        case 0x10:
            switch control {
            default:
                self.lastint = big.NewInt(int64(control & 0xf))
            case 0x1b:
                self.lastint = self.unsigned_to_signed(self.decode_int(4), 32)
            case 0x1c:
                self.lastint = self.unsigned_to_signed(self.decode_int(2), 16)
            case 0x1d:
                self.lastint = self.unsigned_to_signed(self.decode_int(1), 8)
            case 0x1e:
                self.lastint = new(big.Int).Neg(self.decode_int(0))
            case 0x1f:
                self.lastint = self.decode_int(0)
            }
            return self.lastint
        // Floating point numbers
        case 0x20:
            switch control {
            case 0x20:
                return math.NaN()
            case 0x2b:
                self.store_err(&UnmarshalTypeError{
                    "float80",
                    reflect.TypeOf(0),
                    self.readcount,
                })
                return math.NaN()
            case 0x2c: {
                var result float64
                self.store_err(binary.Read(self.reader, binary.BigEndian, &result))
                self.readcount += 8
                return result
            }
            case 0x2d: {
                var result float32
                self.store_err(binary.Read(self.reader, binary.BigEndian, &result))
                self.readcount += 4
                return result
            }
            case 0x2e:
                return math.Inf(-1)
            case 0x2f:
                return math.Inf(1)
            }
        // UTF-16 strings
        case 0x30:
            switch control {
            default:
                return self.load_string_utf16le(uint(control & 0xf))
            case 0x3c: {
                hashvalue, err := self.reader.ReadByte()
                self.store_err(err)
                if err != nil {
                    return ""
                }
                self.readcount++
                if self.texthash[hashvalue] != nil {
                    return *self.texthash[hashvalue]
                } else {
                    self.store_err(&SyntaxError{
                        fmt.Sprintf("JKSN stream requires a non-existing hash: 0x%02x", hashvalue),
                        self.readcount,
                    })
                    return ""
                }
            }
            case 0x3d:
                return self.load_string_utf16le(uint(self.decode_int(2).Uint64()))
            case 0x3e:
                return self.load_string_utf16le(uint(self.decode_int(1).Uint64()))
            case 0x3f:
                return self.load_string_utf16le(uint(self.decode_int(0).Uint64()))
            }
        // UTF-8 strings
        case 0x40:
            switch control {
            default:
                return self.load_string_utf8(uint(control & 0xf))
            case 0x4d:
                return self.load_string_utf8(uint(self.decode_int(2).Uint64()))
            case 0x4e:
                return self.load_string_utf8(uint(self.decode_int(1).Uint64()))
            case 0x4f:
                return self.load_string_utf8(uint(self.decode_int(0).Uint64()))
            }
        // Blob strings
        case 0x50:
            switch control {
            default:
                return self.load_bytes(uint(control & 0xf))
            case 0x5c: {
                hashvalue, err := self.reader.ReadByte()
                self.store_err(err)
                if err != nil {
                    return ""
                }
                self.readcount++
                if self.blobhash[hashvalue] != nil {
                    result := make([]byte, len(self.blobhash[hashvalue]))
                    copy(result, self.blobhash[hashvalue])
                    return result
                } else {
                    self.store_err(&SyntaxError{
                        fmt.Sprintf("JKSN stream requires a non-existing hash: 0x%02x", hashvalue),
                        self.readcount,
                    })
                    return []byte("")
                }
            }
            case 0x5d:
                return self.load_bytes(uint(self.decode_int(2).Uint64()))
            case 0x5e:
                return self.load_bytes(uint(self.decode_int(1).Uint64()))
            case 0x5f:
                return self.load_bytes(uint(self.decode_int(0).Uint64()))
            }
        // Hashtable refreshers
        case 0x70:
            switch control {
            case 0x70:
                for i := range self.texthash {
                    self.texthash[i] = nil
                }
                for i := range self.blobhash {
                    self.blobhash[i] = nil
                }
            default: {
                count := control & 0xf
                for count != 0 {
                    self.load_value()
                    count--
                }
            }
            case 0x7d: {
                count := self.decode_int(2).Uint64()
                for count != 0 {
                    self.load_value()
                    count--
                }
            }
            case 0x7e: {
                count := self.decode_int(1).Uint64()
                for count != 0 {
                    self.load_value()
                    count--
                }
            }
            case 0x7f: {
                count := self.decode_int(0)
                one := big.NewInt(1)
                for count.Sign() > 0 {
                    self.load_value()
                    count.Sub(count, one)
                }
            }
            }
            continue
        // Arrays
        case 0x80: {
            var length uint64
            switch control {
            default:
                length = uint64(control & 0xf)
            case 0x8d:
                length = self.decode_int(2).Uint64()
            case 0x8e:
                length = self.decode_int(1).Uint64()
            case 0x8f:
                length = self.decode_int(0).Uint64()
            }
            result := make([]interface{}, length)
            for i := range result {
                result[i] = self.load_value()
            }
            return result
        }
        // Objects
        case 0x90: {
            var length uint64
            switch control {
            default:
                length = uint64(control & 0xf)
            case 0x9d:
                length = self.decode_int(2).Uint64()
            case 0x9e:
                length = self.decode_int(1).Uint64()
            case 0x9f:
                length = self.decode_int(0).Uint64()
            }
            result := make(map[interface{}]interface{}, length)
            for i := uint64(0); i < length; i++ {
                key := self.load_value()
                result[key] = self.load_value()
            }
            return result
        }
        // Row-col swapped arrays
        case 0xa0: {
            var length uint
            switch control {
            case 0xa0:
                return unspecified_value
            default:
                length = uint(control & 0xf)
            case 0xad:
                length = uint(self.decode_int(2).Uint64())
            case 0xae:
                length = uint(self.decode_int(1).Uint64())
            case 0xaf:
                length = uint(self.decode_int(0).Uint64())
            }
            return self.load_swapped_array(length)
        }
        case 0xc0 :
            switch control {
            // Lengthless arrays
            case 0xc8: {
                result := make([]interface{}, 0)
                for {
                    item := self.load_value()
                    switch item.(type) {
                    default:
                        result = append(result, item)
                    case unspecified:
                        return result
                    }
                }
            }
            // Padding byte
            case 0xca:
                continue
            }
        // Delta encoded integers
        case 0xd0: {
            var delta *big.Int
            switch control {
            case 0xd0, 0xd1, 0xd2, 0xd3, 0xd4, 0xd5:
                delta = big.NewInt(int64(control & 0xf))
            case 0xd6, 0xd7, 0xd8, 0xd9, 0xda:
                delta = big.NewInt(int64(control & 0xf) - 11)
            case 0xdb:
                delta = self.decode_int(4)
            case 0xdc:
                delta = self.decode_int(2)
            case 0xdd:
                delta = self.decode_int(1)
            case 0xde:
                delta = self.decode_int(0)
                delta.Neg(delta)
            case 0xdf:
                delta = self.decode_int(0)
            }
            if self.lastint != nil {
                self.lastint.Add(self.lastint, delta)
                return self.lastint
            } else {
                self.store_err(&SyntaxError{
                    "JKSN stream contains an invalid delta encoded integer",
                    self.readcount,
                })
                return new(big.Int)
            }
        }
        case 0xf0:
            if control <= 0xf5 {
                switch control {
                case 0xf0:
                    self.reader.Discard(1)
                    self.readcount++
                case 0xf1:
                    self.reader.Discard(4)
                    self.readcount += 4
                case 0xf2:
                    self.reader.Discard(16)
                    self.readcount += 16
                case 0xf3:
                    self.reader.Discard(20)
                    self.readcount += 20
                case 0xf4:
                    self.reader.Discard(32)
                    self.readcount += 32
                case 0xf5:
                    self.reader.Discard(64)
                    self.readcount += 64
                }
                continue
            } else if control >= 0xf8 && control <= 0xfd {
                result := self.load_value()
                switch control {
                case 0xf8:
                    self.reader.Discard(1)
                    self.readcount++
                case 0xf9:
                    self.reader.Discard(4)
                    self.readcount += 4
                case 0xfa:
                    self.reader.Discard(16)
                    self.readcount += 16
                case 0xfb:
                    self.reader.Discard(20)
                    self.readcount += 20
                case 0xfc:
                    self.reader.Discard(32)
                    self.readcount += 32
                case 0xfd:
                    self.reader.Discard(64)
                    self.readcount += 64
                }
                return result
            } else if control == 0xff {
                self.load_value()
                continue
            }
        }
        self.store_err(&SyntaxError{
            fmt.Sprintf("jksn: cannot decode JKSN from byte 0x%02x", control),
            self.readcount-1,
        })
        return nil
    }
}

func (self *Decoder) load_string_utf8(length uint) string {
    buf := make([]byte, length)
    n, err := io.ReadFull(self.reader, buf)
    self.store_err(err)
    self.readcount += int64(n)
    res := string(buf)
    self.texthash[djb_hash(buf)] = &res
    return res
}

func (self *Decoder) load_string_utf16le(length uint) string {
    buf := make([]byte, length*2)
    n, err := io.ReadFull(self.reader, buf)
    self.store_err(err)
    self.readcount += int64(n)
    res := utf16le_to_utf8(buf)
    self.texthash[djb_hash(buf)] = &res
    return res
}

func (self *Decoder) load_bytes(length uint) []byte {
    buf := make([]byte, length)
    n, err := io.ReadFull(self.reader, buf)
    self.store_err(err)
    self.readcount += int64(n)
    self.blobhash[djb_hash(buf)] = buf
    res := make([]byte, length)
    copy(res, buf)
    return res
}

func (self *Decoder) load_swapped_array(column_length uint) (result []map[interface{}]interface{}) {
    for i := uint(0); i < column_length; i++ {
        column_name := self.load_value()
        column_values_general := self.load_value()
        if column_values, ok := column_values_general.([]interface{}); ok {
            for idx, value := range column_values {
                if idx == len(result) {
                    result = append(result, make(map[interface{}]interface{}))
                }
                switch value.(type) {
                case unspecified:
                default:
                    result[idx][column_name] = value
                }
            }
        }
    }
    return
}

func (self *Decoder) fit_type(obj interface{}, generic_value interface{}) {
    if obj_generic, ok := obj.(*interface{}); ok {
        *obj_generic = generic_value
    } else {
        panic("jksn: unimplemented")
    }
}

func (self *Decoder) decode_int(size uint) *big.Int {
    if size == 1 {
        int_byte, err := self.reader.ReadByte()
        self.store_err(err)
        self.readcount++
        return big.NewInt(int64(int_byte))
    } else if size == 2 {
        var buf [2]byte
        n, err := io.ReadFull(self.reader, buf[:])
        self.store_err(err)
        self.readcount += int64(n)
        return big.NewInt(int64(buf[0]) << 8 | int64(buf[1]))
    } else if size == 4 {
        var buf [4]byte
        n, err := io.ReadFull(self.reader, buf[:])
        self.store_err(err)
        self.readcount += int64(n)
        return big.NewInt(int64(buf[0]) << 24 | int64(buf[1]) << 16 | int64(buf[2]) << 8 | int64(buf[3]))
    } else if size == 0 {
        result := new(big.Int)
        thisbyte := uint8(0xff)
        for thisbyte & 0x80 != 0 {
            var err error
            thisbyte, err = self.reader.ReadByte()
            self.store_err(err)
            self.readcount++
            if err != nil {
                return new(big.Int)
            }
            result.Lsh(result, 7)
            result.Or(result, big.NewInt(int64(thisbyte & 0x7f)))
        }
        return result
    } else {
        panic("jksn: size not in (1, 2, 4, 0)")
    }
    return new(big.Int)
}

func (self *Decoder) unsigned_to_signed(x *big.Int, bits uint) *big.Int {
    // return x - ((x >> (bits - 1)) << bits)
    temp := new(big.Int)
    temp.Rsh(x, bits-1)
    temp.Lsh(temp, bits)
    return temp.Sub(x, temp)
}

func (self *Decoder) store_err(err error) error {
    if self.firsterr == nil {
        self.firsterr = err
    }
    return self.firsterr
}

func utf8_to_utf16le(utf8str string) []byte {
    utf16str := utf16.Encode([]rune(utf8str))
    utf16lestr := make([]byte, len(utf16str)*2)
    for i, j := 0, 0; i < len(utf16str); i, j = i+1, j+2 {
        utf16lestr[j] = uint8(utf16str[i]);
        utf16lestr[j+1] = uint8(utf16str[i] >> 8);
    }
    return utf16lestr
}

func utf16le_to_utf8(utf16lestr []byte) string {
    if (len(utf16lestr) & 0x1) != 0 {
        panic("jksn: len(utf16lestr) not even")
    }
    utf16str := make([]uint16, len(utf16lestr)/2)
    for i, j := 0, 0; i < len(utf16lestr); i, j = i+2, j+1 {
        utf16str[j] = uint16(utf16lestr[i]) + (uint16(utf16lestr[i+1]) << 8)
    }
    return string(utf16.Decode(utf16str))
}

func djb_hash(obj []byte) (result uint8) {
    for _, i := range obj {
        result += (result << 5) + i
    }
    return
}

