# Kitchen sink

Text with *emphasis*, **strong**, `code`, ~~strike~~ and a [link](https://example.com "Title").
A soft break, and a hard one
here. Entities &amp; escapes \* and &copy; and a {string|Buffer} annotation.

> Stability: 2 - Stable
>
> A second paragraph in the quote.

* A tight
* list with `items`
  * and nesting

1. A loose

2. ordered list

- [ ] a task
- [x] done

```js displayName="example"
const a = 1;
```

    indented code

<!-- YAML
added: v1.0.0
-->

<div>raw html</div>

| Left | Center | Right |
| :--- | :----: | ----: |
| a | `b\|c` | 1 |

Reference [style][ref] links and a footnote[^note] and www.example.com and an
email someone@example.com but not pkg@1.2.3.

![An image](img.png) and ![ref image][img]

***

[ref]: https://example.com/ref
[img]: https://example.com/img.png
[^note]: The footnote text.
