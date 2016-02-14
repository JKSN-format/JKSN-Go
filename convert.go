package main

import (
    "encoding/json"
    "fmt"
    "os"
    "reflect"
    "./jksn"
)

func main() {
    is_encoding := true
    for _, arg := range os.Args {
        if arg == "-d" {
            is_encoding = false
        }
    }

    if is_encoding {
        json_decoder := json.NewDecoder(os.Stdin)
        var value interface{}
        err := json_decoder.Decode(&value)
        if err != nil {
            panic(err.Error())
        }
        jksn_encoder := jksn.NewEncoder(os.Stdout)
        err = jksn_encoder.Encode(value)
        if err != nil {
            panic(err.Error())
        }
    } else {
        jksn_decoder := jksn.NewDecoder(os.Stdin)
        var value interface{}
        err := jksn_decoder.Decode(&value)
        if err != nil {
            panic(err.Error())
        }
        filter_map_key(&value)
        json_encoder := json.NewEncoder(os.Stdout)
        err = json_encoder.Encode(value)
        if err != nil {
            panic(err.Error())
        }
    }
}

func filter_map_key(obj *interface{}) {
    if *obj == nil {
        return
    }
    value := reflect.ValueOf(*obj)
    switch value.Kind() {
    case reflect.Array, reflect.Slice:
        filtered := make([]interface{}, value.Len())
        for i := 0; i < value.Len(); i++ {
            new_value := value.Index(i).Interface()
            filter_map_key(&new_value)
            filtered[i] = new_value
        }
        *obj = filtered
    case reflect.Map:
        keys := value.MapKeys()
        filtered := make(map[string]interface{}, len(keys))
        for _, key := range keys {
            key_str := fmt.Sprintf("%v", key.Interface())
            new_value := value.MapIndex(key).Interface()
            filter_map_key(&new_value)
            filtered[key_str] = new_value
        }
        *obj = filtered
    }
}
