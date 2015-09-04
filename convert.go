package main

import (
    "encoding/json"
    "os"
    "./jksn"
)

func main() {
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
}
