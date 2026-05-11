package pdf

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type sessionPersistenceXRef interface {
	GetNumObjects() int
	GetTrailer() (*entity.Dict, error)
	RawData() []byte
	StartXRefOffset() (uint64, error)
}

// SaveWithEmbeddedSession writes a new PDF file with session state embedded
// via incremental update objects.
func (d *Document) SaveWithEmbeddedSession(path string) error {
	state, err := d.ExportSessionState()
	if err != nil {
		return err
	}

	x, ok := d.doc.XRef().(sessionPersistenceXRef)
	if !ok {
		return fmt.Errorf("document xref does not support persistence")
	}

	raw := x.RawData()
	if len(raw) == 0 {
		return fmt.Errorf("empty PDF stream")
	}

	prevXRef, err := x.StartXRefOffset()
	if err != nil {
		return fmt.Errorf("resolve startxref: %w", err)
	}

	trailer, err := x.GetTrailer()
	if err != nil {
		return fmt.Errorf("load trailer: %w", err)
	}

	rootObj := trailer.GetRaw(entity.Name("/Root"))
	if rootObj == nil {
		rootObj = trailer.GetRaw(entity.Name("Root"))
	}
	rootRef, ok := rootObj.(entity.Ref)
	if !ok {
		return fmt.Errorf("trailer /Root is not reference")
	}

	nextObjNum := x.GetNumObjects()
	if nextObjNum < 1 {
		nextObjNum = 1
	}
	sessionObjNum := nextObjNum

	encodedState := base64.StdEncoding.EncodeToString(state)
	sessionObject := fmt.Sprintf(
		"<< /Type /GoPDFSession /Version 1 /Encoding /Base64 /Data (%s) >>",
		encodedState,
	)

	var out bytes.Buffer
	out.Write(raw)
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		out.WriteByte('\n')
	}

	sessionOffset := out.Len()
	fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", sessionObjNum, sessionObject)

	xrefOffset := out.Len()
	fmt.Fprintf(&out, "xref\n%d 1\n", sessionObjNum)
	fmt.Fprintf(&out, "%010d 00000 n \n", sessionOffset)

	fmt.Fprintf(
		&out,
		"trailer\n<< /Size %d /Root %d %d R /Prev %d /GoPDFSessionState %d 0 R >>\n",
		sessionObjNum+1,
		rootRef.Num(),
		rootRef.Gen(),
		prevXRef,
		sessionObjNum,
	)
	fmt.Fprintf(&out, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write PDF: %w", err)
	}

	return nil
}

func (d *Document) applyEmbeddedSessionState() {
	if x, ok := d.doc.XRef().(interface{ GetTrailer() (*entity.Dict, error) }); ok {
		if trailer, err := x.GetTrailer(); err == nil && trailer != nil {
			sessionObj := trailer.Get(entity.Name("GoPDFSessionState"))
			if sessionObj == nil {
				sessionObj = trailer.Get(entity.Name("/GoPDFSessionState"))
			}
			if sessionObj != nil {
				data := d.extractEmbeddedSessionData(sessionObj)
				if len(data) > 0 {
					if err := d.ImportSessionState(data); err == nil {
						return
					}
				}
			}
		}
	}

	catalog := d.doc.Catalog()
	if catalog == nil {
		return
	}
	sessionObj := catalog.Get(entity.Name("GoPDFSessionState"))
	if sessionObj == nil {
		sessionObj = catalog.Get(entity.Name("/GoPDFSessionState"))
	}
	if sessionObj == nil {
		return
	}

	data := d.extractEmbeddedSessionData(sessionObj)
	if len(data) == 0 {
		return
	}
	if err := d.ImportSessionState(data); err != nil {
		return
	}
}

func (d *Document) extractEmbeddedSessionData(obj entity.Object) []byte {
	switch v := obj.(type) {
	case *entity.String:
		return []byte(v.Value())
	case *entity.Stream:
		decoded, err := v.Decode()
		if err != nil {
			return nil
		}
		return decoded
	case *entity.Dict:
		if extractEntityNameOrString(v.Get(entity.Name("Encoding"))) != "Base64" {
			return nil
		}
		data := extractEntityString(v.Get(entity.Name("Data")))
		if data == "" {
			return nil
		}
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil
		}
		return decoded
	case entity.Ref:
		fetched, err := d.doc.XRef().Fetch(v)
		if err != nil {
			return nil
		}
		return d.extractEmbeddedSessionData(fetched)
	default:
		return nil
	}
}
