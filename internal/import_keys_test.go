package internal

func resetLibraryCatalogIndexForTest() {
	libraryCatalogKeys.mu.Lock()
	defer libraryCatalogKeys.mu.Unlock()
	libraryCatalogKeys.byKey = map[string]string{}
}
