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
    "io"
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
    origin      interface{}
    control     uint8
    data        []byte
    buf         []byte
    children    *list.List
    hash        *uint8
}

func new_jksn_proxy(origin interface{}, control uint8, data []byte, buf []byte) (res *jksn_proxy) {
    res.origin = origin
    res.control = control
    res.data = make([]byte, len(data))
    copy(res.data, data)
    res.buf = make([]byte, len(buf))
    copy(res.data, buf)
    res.children = list.New()
    return
}

func (self *jksn_proxy) output(fp io.Writer, recursive bool) (err error) {
    control := [1]byte{self.control}
    _, err = fp.Write(control[:])
    if err != nil { return }
    _, err = fp.Write(self.data)
    if err != nil { return }
    _, err = fp.Write(self.buf)
    if err != nil { return }
    if recursive {
        for i := self.children.Front(); i != nil; i = i.Next() {
            err = i.Value.(*jksn_proxy).output(fp, true)
            if err != nil { return }
        }
    }
    return
}

func (self *jksn_proxy) size(depth uint) (result int64) {
    result = 1 + int64(len(self.data)) + int64(len(self.buf))
    if depth == 0 {
        for i := self.children.Front(); i != nil; i = i.Next() {
            result += i.Value.(*jksn_proxy).size(0);
        }
    } else if depth != 1 {
        for i := self.children.Front(); i != nil; i = i.Next() {
            result += i.Value.(*jksn_proxy).size(depth-1);
        }
    }
    return
}

type unspecified_value struct {}

type Encoder struct {
    writer io.Writer
    firsterr error
    lastint big.Int
    texthash [256][]byte
    blobhash [256][]byte
}

func NewEncoder(writer io.Writer) (res *Encoder) {
    res.writer = writer
    return
}

func (self *Encoder) Encode(obj interface{}) (err error) {
    self.firsterr = nil
    result := self.dump_to_proxy(obj)
    _, err = self.writer.Write([]byte("jk!"))
    if err == nil {
        err = result.output(self.writer, true)
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
        if value.IsNil() {
            return self.dump_nil(nil)
        } else {
            value = reflect.Indirect(value)
        }
        switch value.Kind() {
            case reflect.Bool:
                return self.dump_bool(value.Bool())
            default:
                self.firsterr = &UnsupportedTypeError{ value.Type() }
                return self.dump_nil(nil)
        }
    }
}

func (self *Encoder) dump_nil(obj interface{}) *jksn_proxy {
    return new_jksn_proxy(obj, 0x01, []byte{}, []byte{})
}

func (self *Encoder) dump_unspecified(obj unspecified_value) *jksn_proxy {
    return new_jksn_proxy(obj, 0xa0, []byte{}, []byte{})
}

func (self *Encoder) dump_bool(obj bool) *jksn_proxy {
    if obj {
        return new_jksn_proxy(obj, 0x03, []byte{}, []byte{})
    } else {
        return new_jksn_proxy(obj, 0x02, []byte{}, []byte{})
    }
}
