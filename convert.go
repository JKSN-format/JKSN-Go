package main

import (
    "encoding/json"
    "flag"
    "os"
    "./jksn"
)

func main() {
    is_encoding := true
    for _, arg := range flag.Args() {
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
        json_encoder := json.NewEncoder(os.Stdout)
        err = json_encoder.Encode(value)
        if err != nil {
            panic(err.Error())
        }
    }
}
