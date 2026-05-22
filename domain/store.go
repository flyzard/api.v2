package domain

// IssuedDocumentReader is the read-side port for cross-document validation
// (e.g. ND product-set checks against the originating invoice). Concrete
// implementations live in the persistence layer; the domain never sees a
// database.
//
// Keep this interface to a single method until a real consumer needs more —
// speculative methods couple the domain to storage shapes we don't yet have.
type IssuedDocumentReader interface {
	FindByNumber(DocNumber) (IssuedDocument, error)
}
