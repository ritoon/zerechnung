package main

import (
	"fmt"
	"html/template"
)

// flagBox is the view box of every flag: 27 × 18, the 3:2 shape of most
// flags of Europe. The others (Switzerland, Belgium, Denmark…) are drawn in
// it too, so that they line up in a table.
const flagBox = "0 0 27 18"

// flags is the flag of each country of the compliance page and of each
// language of the site, keyed by ISO 3166-1 alpha-2 code: the content of an
// SVG image of flagBox. The flags are shown at the size of a letter, so the
// coats of arms are simplified. They are inline images, not emoji, which
// Windows shows as two letters.
var flags = map[string]string{
	// Stripes.
	"AT": `<path fill="#c8102e" d="M0 0h27v18H0z"/><path fill="#fff" d="M0 6h27v6H0z"/>`,
	"BE": `<path d="M0 0h9v18H0z"/><path fill="#fdda24" d="M9 0h9v18H9z"/><path fill="#ef3340" d="M18 0h9v18h-9z"/>`,
	"BG": `<path fill="#fff" d="M0 0h27v6H0z"/><path fill="#00966e" d="M0 6h27v6H0z"/><path fill="#d62612" d="M0 12h27v6H0z"/>`,
	"DE": `<path d="M0 0h27v6H0z"/><path fill="#d00" d="M0 6h27v6H0z"/><path fill="#ffce00" d="M0 12h27v6H0z"/>`,
	"EE": `<path fill="#0072ce" d="M0 0h27v6H0z"/><path d="M0 6h27v6H0z"/><path fill="#fff" d="M0 12h27v6H0z"/>`,
	"FR": `<path fill="#002654" d="M0 0h9v18H0z"/><path fill="#fff" d="M9 0h9v18H9z"/><path fill="#ce1126" d="M18 0h9v18h-9z"/>`,
	"HU": `<path fill="#ce2939" d="M0 0h27v6H0z"/><path fill="#fff" d="M0 6h27v6H0z"/><path fill="#477050" d="M0 12h27v6H0z"/>`,
	"IE": `<path fill="#169b62" d="M0 0h9v18H0z"/><path fill="#fff" d="M9 0h9v18H9z"/><path fill="#ff883e" d="M18 0h9v18h-9z"/>`,
	"IT": `<path fill="#009246" d="M0 0h9v18H0z"/><path fill="#fff" d="M9 0h9v18H9z"/><path fill="#ce2b37" d="M18 0h9v18h-9z"/>`,
	"LT": `<path fill="#fdb913" d="M0 0h27v6H0z"/><path fill="#006a44" d="M0 6h27v6H0z"/><path fill="#c1272d" d="M0 12h27v6H0z"/>`,
	"LU": `<path fill="#ef3340" d="M0 0h27v6H0z"/><path fill="#fff" d="M0 6h27v6H0z"/><path fill="#00a3e0" d="M0 12h27v6H0z"/>`,
	"LV": `<path fill="#9e3039" d="M0 0h27v18H0z"/><path fill="#fff" d="M0 7.2h27v3.6H0z"/>`,
	"NL": `<path fill="#ae1c28" d="M0 0h27v6H0z"/><path fill="#fff" d="M0 6h27v6H0z"/><path fill="#21468b" d="M0 12h27v6H0z"/>`,
	"PL": `<path fill="#fff" d="M0 0h27v9H0z"/><path fill="#dc143c" d="M0 9h27v9H0z"/>`,
	"RO": `<path fill="#002b7f" d="M0 0h9v18H0z"/><path fill="#fcd116" d="M9 0h9v18H9z"/><path fill="#ce1126" d="M18 0h9v18h-9z"/>`,
	"CZ": `<path fill="#fff" d="M0 0h27v9H0z"/><path fill="#d7141a" d="M0 9h27v9H0z"/><path fill="#11457e" d="M0 0l13.5 9L0 18z"/>`,
	"GR": `<path fill="#0d5eaf" d="M0 0h27v18H0z"/><path fill="#fff" d="M10 2h17v2H10zM10 6h17v2H10zM0 10h27v2H0zM0 14h27v2H0zM4 0h2v10H4zM0 4h10v2H0z"/>`,
	"LI": `<path fill="#002b7f" d="M0 0h27v9H0z"/><path fill="#ce1126" d="M0 9h27v9H0z"/><path fill="#ffd83d" d="M4.2 6.8h4.6l.4-3.4-1.5 1.3-1.2-2.2-1.2 2.2-1.5-1.3z"/>`,
	"MT": `<path fill="#fff" d="M0 0h13.5v18H0z"/><path fill="#cf142b" d="M13.5 0H27v18H13.5z"/><path fill="#a0a0a0" stroke="#cf142b" stroke-width=".35" d="M3.6 1.6H5v1.6h1.6v1.4H5v1.6H3.6V4.6H2V3.2h1.6z"/>`,

	// Crosses.
	"CH": `<path fill="#da291c" d="M0 0h27v18H0z"/><path fill="#fff" d="M11.75 3.38h3.5v11.25h-3.5zM7.88 7.25h11.25v3.5H7.88z"/>`,
	"DK": `<path fill="#c8102e" d="M0 0h27v18H0z"/><path fill="#fff" d="M8.7 0h2.8v18H8.7zM0 7.6h27v2.8H0z"/>`,
	"FI": `<path fill="#fff" d="M0 0h27v18H0z"/><path fill="#002f6c" d="M7.5 0H12v18H7.5zM0 6.55h27v4.9H0z"/>`,
	"IS": `<path fill="#02529c" d="M0 0h27v18H0z"/><path fill="#fff" d="M7.56 0h4.32v18H7.56zM0 7h27v4H0z"/><path fill="#dc1e35" d="M8.64 0h2.16v18H8.64zM0 8h27v2H0z"/>`,
	"NO": `<path fill="#ba0c2f" d="M0 0h27v18H0z"/><path fill="#fff" d="M7.36 0h4.91v18H7.36zM0 6.75h27v4.5H0z"/><path fill="#00205b" d="M8.59 0h2.45v18H8.59zM0 7.88h27v2.25H0z"/>`,
	"SE": `<path fill="#006aa7" d="M0 0h27v18H0z"/><path fill="#fecc00" d="M8.4 0h3.4v18H8.4zM0 7.2h27v3.6H0z"/>`,
	// The red saltire is counterchanged: on the hoist side, the broad white
	// is above it.
	"GB": `<path fill="#012169" d="M0 0h27v18H0z"/><path stroke="#fff" stroke-width="3.6" d="M0 0l27 18M27 0 0 18"/><path fill="#c8102e" d="M0 0l13.5 9-.67 1L-.67 1zM0 18l13.5-9 .67 1L.67 19zM27 0 13.5 9l-.67-1L26.33-1zM27 18 13.5 9l.67-1 13.5 9z"/><path fill="#fff" d="M10.5 0h6v18h-6zM0 6h27v6H0z"/><path fill="#c8102e" d="M11.7 0h3.6v18h-3.6zM0 7.2h27v3.6H0z"/>`,

	// Coats of arms and emblems.
	"CY": `<path fill="#fff" d="M0 0h27v18H0z"/><path fill="#d57800" stroke="#d57800" stroke-width=".4" stroke-linejoin="round" d="M7.2 8.6 8.6 7.3l2.4-.7 4-.4 5-2-2.8 2.6-.6 1.4-2.1 1.4-2.5.4-2.8-.2-1.8-.7z"/><path fill="none" stroke="#4e5b31" stroke-width=".6" stroke-linecap="round" d="M9.5 12q1.8 2.4 4 2.6M17.5 12q-1.8 2.4-4 2.6"/><path fill="#4e5b31" d="M10.43 13.06a.6.6 0 0 1-.53-.86.6.6 0 0 1 .53.86zM11.4 13.85a.6.6 0 0 1-.4-.9.6.6 0 0 1 .4.9zM12.43 14.36a.6.6 0 0 1-.18-.91.6.6 0 0 1 .18.91zM10.43 13.06a.6.6 0 0 1-.45.75.6.6 0 0 1 .45-.75zM11.4 13.85a.6.6 0 0 1-.25.8.6.6 0 0 1 .25-.8zM16.57 13.06a.6.6 0 0 1 .53-.86.6.6 0 0 1-.53.86zM15.6 13.85a.6.6 0 0 1 .4-.9.6.6 0 0 1-.4.9zM14.57 14.36a.6.6 0 0 1 .18-.91.6.6 0 0 1-.18.91zM16.57 13.06a.6.6 0 0 1 .45.75.6.6 0 0 1-.45-.75zM15.6 13.85a.6.6 0 0 1 .25.8.6.6 0 0 1-.25-.8z"/>`,
	"ES": `<path fill="#aa151b" d="M0 0h27v18H0z"/><path fill="#f1bf00" d="M0 4.5h27v9H0z"/><path fill="#9c9c9c" d="M5.9 6.4h.7V12h-.7zM11.4 6.4h.7V12h-.7z"/><path fill="#c8102e" d="M7.2 6.4h3.6v3.2c0 1.3-.8 2.1-1.8 2.5-1-.4-1.8-1.2-1.8-2.5z"/><path fill="#fff" d="M9 6.4h1.8V9H9z"/><path fill="#f1bf00" d="M7.2 9H9v2.4H7.2z"/><path fill="#b8860b" d="M7.4 6.2l-.2-1.3.9.5.9-.9.9.9.9-.5-.2 1.3z"/>`,
	"HR": `<path fill="#f00" d="M0 0h27v6H0z"/><path fill="#fff" d="M0 6h27v6H0z"/><path fill="#171796" d="M0 12h27v6H0z"/><path fill="#f00" d="M10.5 4.5h6v5a3 3 0 0 1-6 0z"/><path fill="#fff" d="M12 4.5h1.5V6H12zM15 4.5h1.5V6H15zM10.5 6H12v1.5h-1.5zM13.5 6H15v1.5h-1.5zM12 7.5h1.5V9H12zM15 7.5h1.5V9H15zM10.5 9H12v1.5h-1.5zM13.5 9H15v1.5h-1.5zM12 10.5h1.5V12H12zM15 10.5h1.5V12H15z"/>`,
	"PT": `<path fill="#060" d="M0 0h10.8v18H0z"/><path fill="#f00" d="M10.8 0H27v18H10.8z"/><circle cx="10.8" cy="9" r="3.6" fill="none" stroke="#ffd700"/><path fill="#f00" stroke="#fff" stroke-width=".25" d="M9.2 6.6h3.2v2.8a1.6 1.6 0 0 1-3.2 0z"/><path fill="#fff" d="M9.8 7.2h2v2.2a1 1 0 0 1-2 0z"/><path fill="#039" d="M10.55 7.6h.5v.5h-.5zM10.55 8.4h.5v.5h-.5zM10.55 9.2h.5v.5h-.5zM9.95 8.4h.5v.5h-.5zM11.15 8.4h.5v.5h-.5z"/>`,
	"SI": `<path fill="#fff" d="M0 0h27v6H0z"/><path fill="#005da4" d="M0 6h27v6H0z"/><path fill="#ed1c24" d="M0 12h27v6H0z"/><path fill="#005da4" stroke="#ed1c24" stroke-width=".35" d="M4.6 3h4.8v3.6c0 1.6-1.1 2.5-2.4 3-1.3-.5-2.4-1.4-2.4-3z"/><path fill="#fff" d="M5.1 6.9 6.2 5.1l.4.5.4-.9.4.9.4-.5 1.1 1.8z"/><path fill="none" stroke="#fff" stroke-width=".35" d="M5.3 7.6q.43-.3.85 0t.85 0 .85 0 .85 0"/>`,
	"SK": `<path fill="#fff" d="M0 0h27v6H0z"/><path fill="#0b4ea2" d="M0 6h27v6H0z"/><path fill="#ee1c25" d="M0 12h27v6H0z"/><path fill="#fff" d="M5.5 3.4h6.6v6c0 2.6-1.7 3.9-3.3 4.8-1.6-.9-3.3-2.2-3.3-4.8z"/><path fill="#ee1c25" d="M5.95 3.85h5.7v5.5c0 2.3-1.4 3.4-2.85 4.2-1.45-.8-2.85-1.9-2.85-4.2z"/><path fill="#fff" d="M8.45 5h.7v6.3h-.7zM7.6 6.3H10v.6H7.6zM7.1 7.9h3.4v.7H7.1z"/><path fill="#0b4ea2" d="M6.7 11.8a.9.9 0 0 1 1.35-.5.75.75 0 0 1 1.5 0A.9.9 0 0 1 10.9 11.8c-.6.7-1.2 1.1-2.1 1.5-.9-.4-1.5-.8-2.1-1.5z"/>`,
}

// Flag returns the flag of a country as an inline SVG image. The image is
// decorative: the name of the country or of the language follows it.
func (p *page) Flag(code string) (template.HTML, error) {
	f, ok := flags[code]
	if !ok {
		return "", fmt.Errorf("no flag for %q in flags.go", code)
	}
	return template.HTML(`<svg class="flag-icon" viewBox="` + flagBox + `" aria-hidden="true">` + f + `</svg>`), nil
}
