---
'@doc-kit/core': patch
'@node-core/doc-kit': patch
'@node-core/doc-kit-legacy': patch
'@doc-kit/generator-react': patch
---

Fix generators corrupting each other's input and output. `man-page` and `json-simple` no longer remove nodes from the entries other generators of the run read (which crashed `legacy-json` when run with them), and single-threaded chunks work on their own copy of their items, like worker threads do. Parallel results now come in the order of the input, so `apilinks.json`, `all.html` and `all.json` no longer change from run to run. Template values are inserted literally, so `$$` and `$&` in content (such as the Makefile example on the addons page) are no longer rewritten, and `all.html` lists every module in its side navigation again.
