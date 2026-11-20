package theme

// Additional stroke icons used by the editor, group manager, trash, and import views.
var (
	IconChevronUp   = svg(`<path d="m6 15 6-6 6 6"/>`)
	IconChevronDown = svg(`<path d="m6 9 6 6 6-6"/>`)
	IconTag         = svg(`<path d="M12 2H2v10l9.29 9.29a1 1 0 0 0 1.42 0l7.58-7.58a1 1 0 0 0 0-1.42L12 2Z"/><circle cx="7" cy="7" r="1.2" fill="currentColor"/>`)
	IconFile        = svg(`<path d="M6 3h8l4 4v14H6Z"/><path d="M14 3v4h4"/>`)
	IconPromote     = svg(`<path d="M7 17 17 7M8 7h9v9"/>`)
)
