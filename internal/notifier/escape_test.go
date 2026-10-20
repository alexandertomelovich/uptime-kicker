// Тесты для EscapeHTML: экранирование значений, подставляемых в сообщения
// с ParseMode=HTML.
package notifier

import "testing"

func TestEscapeHTML(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"обычный текст не меняется", "example.com", "example.com"},
		{"открывающий и закрывающий тег", "<b>", "&lt;b&gt;"},
		{"амперсанд", "Tom & Jerry", "Tom &amp; Jerry"},
		{"смешанный ввод", `<a href="x">Tom & Jerry's</a>`, `&lt;a href="x"&gt;Tom &amp; Jerry's&lt;/a&gt;`},
		{"кавычки не экранируются", `"quoted" 'text'`, `"quoted" 'text'`},
		{"кириллица сохраняется", "Сайт <b>β</b>", "Сайт &lt;b&gt;β&lt;/b&gt;"},
		{"пустая строка", "", ""},
		// Функция ожидает исходный текст: уже экранированные сущности
		// экранируются повторно, поэтому вызывать её дважды нельзя.
		{"уже экранированный текст", "&lt;b&gt;", "&amp;lt;b&amp;gt;"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EscapeHTML(tc.in); got != tc.want {
				t.Errorf("EscapeHTML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
