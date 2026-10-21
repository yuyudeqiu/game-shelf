# Third-party notices

## psn-api

PSN request formats and authentication flow are adapted from
https://github.com/achievements-app/psn-api (reference commit
1e9d9a806fcd884a4ddee8086bfe92594611da9f). No package code is executed at runtime.

MIT License

Copyright (c) 2021 Wes Copeland.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.

## Epic launcher protocol reference

Epic launcher endpoint names, public client identifiers and wire fields were
checked against Legendary (https://github.com/legendary-gl/legendary), commit
42f6bdeadde3a9526dc8eb713763476999ac217b. Legendary is distributed under GPLv3.
Game Shelf independently implements these network protocols in Go; no Legendary
source code is copied, translated, bundled, linked or executed at runtime.
Epic artwork and other platform content remain the property of their owners.

Epic playtime endpoint and wire semantics were also checked against the Playnite
Epic library integration (https://github.com/JosefNemec/PlayniteExtensions), commit
886468f65724e6fd104f3a09847a17ba57cfb430. Only
protocol facts were consulted; no Playnite source code is bundled or translated.
