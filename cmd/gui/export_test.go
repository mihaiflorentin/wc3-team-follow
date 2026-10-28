package gui

// SetOpenURL replaces how the bridge opens a web page, for tests.
func (b *Bridge) SetOpenURL(fn func(link string) error) { b.openURL = fn }
