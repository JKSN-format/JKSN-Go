JKSN-Go
=======

This is the Go implementation of [JKSN](https://github.com/JKSN-format/JKSN) Compressed Serialize Notation.

It is not the one which produces the smallest JKSN stream, neither the one which does the fastest processing.

It does not provide the full functionality of JKSN. See the [Python implementation](https://github.com/JKSN-format/JKSN/tree/master/python) for more functionality.

Currently there may be some bugs. Feel free to report them.

### Usage

You can use `jksn.go` as a module. `Marshal` and `Unmarshal` are the most common functions.

They work almost in the same way as the standard `encoding/json` module.

The documentation is not yet complete. But you may understand how it works by reading the source code.

### License

This program is licensed under BSD license.

Copyright (c) 2014-2016 StarBrilliant &lt;m13253@hotmail.com&gt;.
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

THIS SOFTWARE IS PROVIDED **AS IS** AND WITHOUT ANY EXPRESS OR
IMPLIED WARRANTIES, INCLUDING, WITHOUT LIMITATION, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE.
