package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"

	json "github.com/goccy/go-json"
)

const (
	defaultMaxUploadSize = 10 << 20  // 10 MB
	maxUploadSizeLimit   = 100 << 20 // 100 MB hard cap
)

// UploadResponse is the JSON response for a single uploaded file.
type UploadResponse struct {
	Key       string `json:"key"`
	Format    string `json:"format"`
	Converted bool   `json:"converted"`
	Doc       Doc    `json:"document"`
}

// UploadBatchResponse is the JSON response for multi-file upload.
type UploadBatchResponse struct {
	Added   int              `json:"added"`
	Updated int              `json:"updated"`
	Failed  int              `json:"failed"`
	Errors  []string         `json:"errors,omitempty"`
	Results []UploadResponse `json:"results,omitempty"`
}

// handleUpload handles POST /v1/upload (multipart/form-data).
//
// Form fields:
//
//	file / files[]  – one or more files (required)
//	collection      – target collection (required)
//	lang            – document language (required)
//	key             – document key; derived from filename when empty (optional)
//	meta            – JSON-encoded metadata map (optional)
//	ttl             – TTL in seconds (optional)
//	maxSize         – per-file size limit in bytes (optional, default 10 MB, max 100 MB)
//
// Supported file types: .md .txt .html .htm .pdf .docx .odt .rtf .yaml .yml .log .lex .tex .latex
// Default (md/txt) is stored as-is; others are converted to markdown.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Determine max upload size
	maxSize := int64(defaultMaxUploadSize)
	if v := r.FormValue("maxSize"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			maxSize = n
		}
	}
	if maxSize > maxUploadSizeLimit {
		maxSize = maxUploadSizeLimit
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSize*10) // account for multipart overhead

	if err := r.ParseMultipartForm(maxSize); err != nil {
		bad(w, fmt.Errorf("parse multipart: %w", err))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	collection := r.FormValue("collection")
	lang := r.FormValue("lang")
	if collection == "" || lang == "" {
		bad(w, errors.New("missing required fields: collection, lang"))
		return
	}

	// Check write permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	// Parse optional meta JSON
	var meta map[string][]string
	if raw := r.FormValue("meta"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &meta); err != nil {
			bad(w, fmt.Errorf("invalid meta JSON: %w", err))
			return
		}
	}

	keyOverride := r.FormValue("key")

	var ttl int64
	if v := r.FormValue("ttl"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			ttl = n
		}
	}

	// Collect files from both "file" and "files[]" field names
	var files []*multipart.FileHeader
	for _, name := range []string{"file", "files[]", "files"} {
		if fhs, found := r.MultipartForm.File[name]; found {
			files = append(files, fhs...)
		}
	}

	if len(files) == 0 {
		bad(w, errors.New("no files uploaded; use field name 'file' or 'files[]'"))
		return
	}

	// Single file → simple response; multiple → batch response
	if len(files) == 1 {
		res, err := s.processUploadedFile(files[0], collection, lang, keyOverride, meta, ttl, maxSize)
		if err != nil {
			bad(w, err)
			return
		}
		s.Metrics.IncOp("upload")
		ok(w, res)
		return
	}

	// Multi-file upload
	resp := UploadBatchResponse{}
	for _, fh := range files {
		res, err := s.processUploadedFile(fh, collection, lang, "", meta, ttl, maxSize)
		if err != nil {
			resp.Failed++
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: %s", fh.Filename, err.Error()))
			continue
		}
		resp.Results = append(resp.Results, *res)
		resp.Added++ // simplified: treats all as adds
	}
	if len(resp.Errors) == 0 {
		resp.Errors = nil
	}

	s.Metrics.IncOp("upload")
	ok(w, resp)
}

// processUploadedFile reads a single multipart file, converts to markdown if needed,
// and stores via addDocument.
func (s *Server) processUploadedFile(fh *multipart.FileHeader, collection, lang, keyOverride string, baseMeta map[string][]string, ttl, maxSize int64) (*UploadResponse, error) {
	if fh.Size > maxSize {
		return nil, fmt.Errorf("file %q exceeds max size (%d bytes)", fh.Filename, maxSize)
	}

	f, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("file %q exceeds max size (%d bytes)", fh.Filename, maxSize)
	}

	ext := strings.ToLower(path.Ext(fh.Filename))
	format := strings.TrimPrefix(ext, ".")
	if format == "htm" {
		format = "html"
	}

	// Convert to markdown based on format
	var contentMD string
	var converted bool

	switch format {
	case "md", "markdown", "txt", "text", "":
		// Already markdown / plain text — store as-is
		contentMD = string(data)
	case "yaml", "yml", "log", "lex":
		// Text-based formats — wrap in code block for structure
		contentMD = "```" + format + "\n" + string(data) + "\n```"
		converted = true
	case "tex", "latex":
		contentMD = texToMarkdown(data)
		converted = true
	case "html":
		contentMD = htmlToMarkdown(data)
		converted = true
	case "pdf":
		contentMD, err = pdfToMarkdown(data)
		if err != nil {
			return nil, fmt.Errorf("pdf conversion: %w", err)
		}
		converted = true
	case "docx":
		contentMD, err = docxToMarkdown(data)
		if err != nil {
			return nil, fmt.Errorf("docx conversion: %w", err)
		}
		converted = true
	case "odt":
		contentMD, err = odtToMarkdown(data)
		if err != nil {
			return nil, fmt.Errorf("odt conversion: %w", err)
		}
		converted = true
	case "rtf":
		contentMD = rtfToMarkdown(data)
		converted = true
	default:
		return nil, fmt.Errorf("unsupported file format: %s (supported: md, txt, html, pdf, docx, odt, rtf, yaml, log, lex, tex)", format)
	}

	// Extract frontmatter for md/txt files
	if !converted {
		fmMeta, body := parseFrontmatter(contentMD)
		if fmMeta != nil {
			contentMD = body
			// Merge frontmatter (base meta overrides)
			if baseMeta == nil {
				baseMeta = fmMeta
			} else {
				for k, v := range fmMeta {
					if _, exists := baseMeta[k]; !exists {
						baseMeta[k] = v
					}
				}
			}
		}
	}

	// Derive key
	key := keyOverride
	if key == "" {
		key = deriveKeyFromFilename(fh.Filename)
	}
	if key == "" {
		return nil, errors.New("cannot derive key from filename; provide key explicitly")
	}

	// Clone meta and add upload metadata
	docMeta := make(map[string][]string)
	for k, v := range baseMeta {
		docMeta[k] = v
	}
	docMeta["upload_format"] = []string{format}
	docMeta["upload_filename"] = []string{fh.Filename}
	if converted {
		docMeta["upload_converted"] = []string{"true"}
	}

	// Validate schema
	if err := s.SchemaManager.Validate(collection, docMeta); err != nil {
		return nil, err
	}

	// Store
	saved, _, err := s.addDocument(collection, key, lang, docMeta, contentMD, ttl, true)
	if err != nil {
		return nil, err
	}

	return &UploadResponse{
		Key:       key,
		Format:    format,
		Converted: converted,
		Doc:       saved,
	}, nil
}

// deriveKeyFromFilename strips the extension and returns a URL-safe key.
func deriveKeyFromFilename(filename string) string {
	base := path.Base(filename)
	ext := path.Ext(base)
	if ext != "" {
		base = base[:len(base)-len(ext)]
	}
	base = strings.TrimSpace(base)
	if base == "" || base == "." {
		return ""
	}
	// Replace spaces with hyphens, lowercase
	base = strings.ToLower(base)
	base = strings.ReplaceAll(base, " ", "-")
	return base
}

// ---------------------------------------------------------------------------
// Lightweight format converters (zero external dependencies)
// ---------------------------------------------------------------------------

// htmlToMarkdown does a best-effort HTML→Markdown conversion.
// It handles common elements: headings, paragraphs, links, lists, bold/italic, code.
func htmlToMarkdown(data []byte) string {
	s := string(data)

	// Remove <script> and <style> blocks
	s = stripTagBlock(s, "script")
	s = stripTagBlock(s, "style")

	// Headings
	for i := 6; i >= 1; i-- {
		prefix := strings.Repeat("#", i) + " "
		tag := fmt.Sprintf("h%d", i)
		s = replaceTagContent(s, tag, prefix, "\n\n")
	}

	// Bold / italic
	s = replaceTagContent(s, "strong", "**", "**")
	s = replaceTagContent(s, "b", "**", "**")
	s = replaceTagContent(s, "em", "*", "*")
	s = replaceTagContent(s, "i", "*", "*")

	// Code
	s = replaceTagContent(s, "code", "`", "`")

	// Links: <a href="url">text</a> → [text](url)
	s = convertLinks(s)

	// List items
	s = replaceTagContent(s, "li", "- ", "\n")

	// Paragraphs / divs → double newline
	s = replaceTagContent(s, "p", "", "\n\n")
	s = replaceTagContent(s, "div", "", "\n\n")

	// Line breaks
	s = replaceAllFold(s, "<br>", "\n")
	s = replaceAllFold(s, "<br/>", "\n")
	s = replaceAllFold(s, "<br />", "\n")

	// Strip remaining HTML tags
	s = stripAllTags(s)

	// Decode common HTML entities
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")

	// Clean up excessive whitespace
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(s)
}

// pdfToMarkdown extracts text from a PDF. Lightweight: reads text between stream/endstream
// and extracts parenthesized text operators (Tj, TJ, '). This is best-effort for simple PDFs.
func pdfToMarkdown(data []byte) (string, error) {
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		return "", errors.New("not a valid PDF file")
	}

	s := string(data)
	var result strings.Builder

	// Extract text from content streams: look for text between ( ) in Tj/TJ operators
	idx := 0
	for {
		streamStart := indexFold(s[idx:], "stream\n")
		if streamStart < 0 {
			streamStart = indexFold(s[idx:], "stream\r\n")
		}
		if streamStart < 0 {
			break
		}
		streamStart += idx

		// Find "stream" keyword end
		contentStart := streamStart + 7 // len("stream\n")
		if s[streamStart+6] == '\r' {
			contentStart = streamStart + 8
		}

		endStream := strings.Index(s[contentStart:], "endstream")
		if endStream < 0 {
			break
		}

		content := s[contentStart : contentStart+endStream]
		extractPDFText(content, &result)

		idx = contentStart + endStream + 9
	}

	text := result.String()
	if strings.TrimSpace(text) == "" {
		return "", errors.New("could not extract text from PDF (scanned/image-based PDFs are not supported; use Docling for advanced parsing)")
	}

	return strings.TrimSpace(text), nil
}

// extractPDFText extracts text from a PDF content stream (between parentheses in text operators).
func extractPDFText(content string, result *strings.Builder) {
	i := 0
	for i < len(content) {
		if content[i] == '(' {
			// Find matching closing paren (handle escaped parens)
			depth := 1
			start := i + 1
			j := start
			for j < len(content) && depth > 0 {
				if content[j] == '\\' {
					j += 2
					continue
				}
				switch content[j] {
				case '(':
					depth++
				case ')':
					depth--
				}
				if depth > 0 {
					j++
				}
			}
			if depth == 0 {
				text := content[start:j]
				// Unescape
				text = strings.ReplaceAll(text, "\\(", "(")
				text = strings.ReplaceAll(text, "\\)", ")")
				text = strings.ReplaceAll(text, "\\\\", "\\")
				text = strings.ReplaceAll(text, "\\n", "\n")
				text = strings.ReplaceAll(text, "\\r", "\r")
				text = strings.ReplaceAll(text, "\\t", "\t")
				if strings.TrimSpace(text) != "" {
					result.WriteString(text)
				}
				i = j + 1
				continue
			}
		}

		// Check for Td/TD (text positioning — likely a new line)
		if i+2 < len(content) && (content[i] == 'T' && (content[i+1] == 'd' || content[i+1] == 'D')) {
			if i+2 >= len(content) || content[i+2] == ' ' || content[i+2] == '\n' || content[i+2] == '\r' {
				result.WriteByte('\n')
			}
		}

		i++
	}
}

// docxToMarkdown extracts text from a DOCX file (which is a ZIP containing XML).
func docxToMarkdown(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("invalid docx file: %w", err)
	}

	// Find word/document.xml
	var docFile *zip.File
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", errors.New("invalid docx: word/document.xml not found")
	}

	rc, err := docFile.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	xmlData, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	return docxXMLToMarkdown(string(xmlData)), nil
}

// docxXMLToMarkdown converts word/document.xml content to markdown.
// Handles paragraphs, runs, headings (pStyle), bold, italic, and lists.
func docxXMLToMarkdown(xml string) string {
	var result strings.Builder

	// Process paragraph by paragraph: <w:p ...>...</w:p>
	remaining := xml
	for {
		pStart := strings.Index(remaining, "<w:p")
		if pStart < 0 {
			break
		}
		pEnd := strings.Index(remaining[pStart:], "</w:p>")
		if pEnd < 0 {
			break
		}
		para := remaining[pStart : pStart+pEnd+6]
		remaining = remaining[pStart+pEnd+6:]

		// Check paragraph style for headings
		headingLevel := 0
		if styleIdx := strings.Index(para, `<w:pStyle w:val="`); styleIdx >= 0 {
			valStart := styleIdx + len(`<w:pStyle w:val="`)
			valEnd := strings.Index(para[valStart:], `"`)
			if valEnd > 0 {
				style := para[valStart : valStart+valEnd]
				// Heading1, Heading2, ... or heading 1, heading 2, ...
				style = strings.ToLower(style)
				if strings.HasPrefix(style, "heading") {
					n := strings.TrimPrefix(style, "heading")
					n = strings.TrimSpace(n)
					if level, err := strconv.Atoi(n); err == nil && level >= 1 && level <= 6 {
						headingLevel = level
					}
				}
			}
		}

		// Check for list (numPr)
		isList := strings.Contains(para, "<w:numPr")

		// Extract text runs: <w:t ...>text</w:t> or <w:t>text</w:t>
		var paraText strings.Builder
		runRemaining := para
		for {
			tStart := strings.Index(runRemaining, "<w:t")
			if tStart < 0 {
				break
			}
			// Skip past the opening tag
			gtIdx := strings.Index(runRemaining[tStart:], ">")
			if gtIdx < 0 {
				break
			}
			textStart := tStart + gtIdx + 1
			tEnd := strings.Index(runRemaining[textStart:], "</w:t>")
			if tEnd < 0 {
				break
			}
			paraText.WriteString(runRemaining[textStart : textStart+tEnd])
			runRemaining = runRemaining[textStart+tEnd+6:]
		}

		text := paraText.String()
		if text == "" && headingLevel == 0 {
			// Empty paragraph → blank line
			result.WriteByte('\n')
			continue
		}

		if headingLevel > 0 {
			result.WriteString(strings.Repeat("#", headingLevel))
			result.WriteByte(' ')
			result.WriteString(text)
			result.WriteString("\n\n")
		} else if isList {
			result.WriteString("- ")
			result.WriteString(text)
			result.WriteByte('\n')
		} else {
			result.WriteString(text)
			result.WriteString("\n\n")
		}
	}

	s := result.String()
	// Clean up excessive newlines
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// HTML helper functions
// ---------------------------------------------------------------------------

// stripTagBlock removes a complete tag block like <script>...</script>.
func stripTagBlock(s, tag string) string {
	for {
		lower := strings.ToLower(s)
		start := strings.Index(lower, "<"+tag)
		if start < 0 {
			return s
		}
		end := strings.Index(lower[start:], "</"+tag+">")
		if end < 0 {
			return s
		}
		s = s[:start] + s[start+end+len("</"+tag+">"):]
	}
}

// replaceTagContent replaces <tag>content</tag> with prefix+content+suffix.
func replaceTagContent(s, tag, prefix, suffix string) string {
	for {
		lower := strings.ToLower(s)
		openTag := "<" + tag
		startIdx := strings.Index(lower, openTag)
		if startIdx < 0 {
			return s
		}
		// Find end of opening tag
		gtIdx := strings.Index(s[startIdx:], ">")
		if gtIdx < 0 {
			return s
		}
		contentStart := startIdx + gtIdx + 1

		closeTag := "</" + tag + ">"
		endIdx := strings.Index(strings.ToLower(s[contentStart:]), closeTag)
		if endIdx < 0 {
			return s
		}
		content := s[contentStart : contentStart+endIdx]
		s = s[:startIdx] + prefix + content + suffix + s[contentStart+endIdx+len(closeTag):]
	}
}

// convertLinks converts <a href="url">text</a> to [text](url).
func convertLinks(s string) string {
	for {
		lower := strings.ToLower(s)
		aStart := strings.Index(lower, "<a ")
		if aStart < 0 {
			return s
		}
		gtIdx := strings.Index(s[aStart:], ">")
		if gtIdx < 0 {
			return s
		}
		openTag := s[aStart : aStart+gtIdx+1]
		contentStart := aStart + gtIdx + 1

		aEnd := strings.Index(strings.ToLower(s[contentStart:]), "</a>")
		if aEnd < 0 {
			return s
		}
		text := s[contentStart : contentStart+aEnd]

		// Extract href
		href := ""
		hrefIdx := strings.Index(strings.ToLower(openTag), `href="`)
		if hrefIdx >= 0 {
			hStart := hrefIdx + 6
			hEnd := strings.Index(openTag[hStart:], `"`)
			if hEnd > 0 {
				href = openTag[hStart : hStart+hEnd]
			}
		}

		if href != "" {
			s = s[:aStart] + "[" + text + "](" + href + ")" + s[contentStart+aEnd+4:]
		} else {
			s = s[:aStart] + text + s[contentStart+aEnd+4:]
		}
	}
}

// stripAllTags removes all remaining HTML tags.
func stripAllTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, ch := range s {
		if ch == '<' {
			inTag = true
			continue
		}
		if ch == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(ch)
		}
	}
	return result.String()
}

// replaceAllFold does a case-insensitive replace.
func replaceAllFold(s, old, new string) string {
	lower := strings.ToLower(s)
	oldLower := strings.ToLower(old)
	var result strings.Builder
	i := 0
	for {
		idx := strings.Index(lower[i:], oldLower)
		if idx < 0 {
			result.WriteString(s[i:])
			break
		}
		result.WriteString(s[i : i+idx])
		result.WriteString(new)
		i += idx + len(old)
	}
	return result.String()
}

// indexFold does a case-insensitive Index.
func indexFold(s, substr string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(substr))
}

// odtToMarkdown extracts text from an ODT file (OpenDocument Text).
// ODT is a ZIP archive containing content.xml with ODF markup.
func odtToMarkdown(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("invalid odt file: %w", err)
	}

	var contentFile *zip.File
	for _, f := range r.File {
		if f.Name == "content.xml" {
			contentFile = f
			break
		}
	}
	if contentFile == nil {
		return "", errors.New("invalid odt: content.xml not found")
	}

	rc, err := contentFile.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	xmlData, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	return odtXMLToMarkdown(string(xmlData)), nil
}

// odtXMLToMarkdown converts ODF content.xml to markdown.
func odtXMLToMarkdown(xml string) string {
	var result strings.Builder

	remaining := xml
	for {
		// Find <text:p or <text:h elements
		pStart := -1
		tag := ""
		for _, t := range []string{"<text:p", "<text:h"} {
			idx := strings.Index(remaining, t)
			if idx >= 0 && (pStart < 0 || idx < pStart) {
				pStart = idx
				tag = t
			}
		}
		if pStart < 0 {
			break
		}

		// Determine closing tag
		var closeTag string
		if tag == "<text:h" {
			closeTag = "</text:h>"
		} else {
			closeTag = "</text:p>"
		}

		// Find end of opening tag
		gtIdx := strings.Index(remaining[pStart:], ">")
		if gtIdx < 0 {
			break
		}
		openTag := remaining[pStart : pStart+gtIdx+1]
		contentStart := pStart + gtIdx + 1

		pEnd := strings.Index(remaining[contentStart:], closeTag)
		if pEnd < 0 {
			break
		}
		content := remaining[contentStart : contentStart+pEnd]
		remaining = remaining[contentStart+pEnd+len(closeTag):]

		// Extract text (strip all XML tags from content)
		text := stripAllTags(content)
		text = strings.TrimSpace(text)

		if tag == "<text:h" {
			// Detect outline level
			level := 1
			if lvlIdx := strings.Index(openTag, `text:outline-level="`); lvlIdx >= 0 {
				lvlStart := lvlIdx + len(`text:outline-level="`)
				if lvlStart < len(openTag) {
					lvlEnd := strings.Index(openTag[lvlStart:], `"`)
					if lvlEnd > 0 {
						if n, err := strconv.Atoi(openTag[lvlStart : lvlStart+lvlEnd]); err == nil && n >= 1 && n <= 6 {
							level = n
						}
					}
				}
			}
			if text != "" {
				result.WriteString(strings.Repeat("#", level))
				result.WriteByte(' ')
				result.WriteString(text)
				result.WriteString("\n\n")
			}
		} else {
			// Check if it's a list item (text:list-item parent)
			if text != "" {
				result.WriteString(text)
				result.WriteString("\n\n")
			} else {
				result.WriteByte('\n')
			}
		}
	}

	s := result.String()
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

// rtfToMarkdown extracts plain text from an RTF file.
// Strips RTF control words and groups, keeping only text content.
func rtfToMarkdown(data []byte) string {
	s := string(data)
	if !strings.HasPrefix(s, "{\\rtf") {
		// Not valid RTF — treat as plain text
		return s
	}

	var result strings.Builder
	depth := 0
	i := 0
	skipGroup := false

	for i < len(s) {
		ch := s[i]

		switch ch {
		case '{':
			depth++
			// Skip special groups like {\fonttbl...}, {\colortbl...}, {\stylesheet...}, {\*\...}
			rest := s[i:]
			if strings.HasPrefix(rest, "{\\fonttbl") ||
				strings.HasPrefix(rest, "{\\colortbl") ||
				strings.HasPrefix(rest, "{\\stylesheet") ||
				strings.HasPrefix(rest, "{\\info") ||
				strings.HasPrefix(rest, "{\\header") ||
				strings.HasPrefix(rest, "{\\footer") ||
				strings.HasPrefix(rest, "{\\*\\") {
				skipGroup = true
			}
			i++
		case '}':
			depth--
			skipGroup = false
			i++
		case '\\':
			if skipGroup {
				// Skip control word inside skipped group
				i++
				for i < len(s) && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z')) {
					i++
				}
				// Skip optional numeric parameter
				if i < len(s) && (s[i] == '-' || (s[i] >= '0' && s[i] <= '9')) {
					i++
					for i < len(s) && s[i] >= '0' && s[i] <= '9' {
						i++
					}
				}
				// Skip delimiter space
				if i < len(s) && s[i] == ' ' {
					i++
				}
				continue
			}
			i++
			if i >= len(s) {
				break
			}
			// Control symbol
			if s[i] == '\'' && i+2 < len(s) {
				// Hex character \'xx
				i += 3
				continue
			}
			if s[i] == '\\' || s[i] == '{' || s[i] == '}' {
				result.WriteByte(s[i])
				i++
				continue
			}
			// Read control word
			wordStart := i
			for i < len(s) && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z')) {
				i++
			}
			word := s[wordStart:i]
			// Skip optional numeric parameter
			if i < len(s) && (s[i] == '-' || (s[i] >= '0' && s[i] <= '9')) {
				i++
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
			}
			// Skip delimiter space
			if i < len(s) && s[i] == ' ' {
				i++
			}
			// Map control words to text
			switch word {
			case "par", "line":
				result.WriteByte('\n')
			case "tab":
				result.WriteByte('\t')
			case "endash":
				result.WriteString("–")
			case "emdash":
				result.WriteString("—")
			case "lquote":
				result.WriteRune('\u2018')
			case "rquote":
				result.WriteRune('\u2019')
			case "ldblquote":
				result.WriteRune('\u201C')
			case "rdblquote":
				result.WriteRune('\u201D')
			case "bullet":
				result.WriteString("- ")
			}
		default:
			if !skipGroup && depth >= 1 {
				result.WriteByte(ch)
			}
			i++
		}
	}

	text := result.String()
	// Clean up
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}

// texToMarkdown converts LaTeX/TeX source to markdown.
// Handles common structural commands: sections, formatting, environments.
func texToMarkdown(data []byte) string {
	s := string(data)

	// Remove comments (lines starting with %)
	lines := strings.Split(s, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "%") {
			continue
		}
		// Strip inline comments (unescaped %)
		for i := 0; i < len(line); i++ {
			if line[i] == '%' && (i == 0 || line[i-1] != '\\') {
				line = line[:i]
				break
			}
		}
		filtered = append(filtered, line)
	}
	s = strings.Join(filtered, "\n")

	// Remove preamble (everything before \begin{document})
	if idx := strings.Index(s, `\begin{document}`); idx >= 0 {
		s = s[idx+len(`\begin{document}`):]
	}
	// Remove \end{document}
	s = strings.ReplaceAll(s, `\end{document}`, "")

	// Remove common preamble commands that might remain
	for _, cmd := range []string{`\documentclass`, `\usepackage`, `\pagestyle`,
		`\setlength`, `\newcommand`, `\renewcommand`, `\input`, `\include`,
		`\bibliographystyle`, `\bibliography`} {
		for {
			idx := strings.Index(s, cmd)
			if idx < 0 {
				break
			}
			// Find end of command (next newline or end of optional/required args)
			end := idx + len(cmd)
			// Skip arguments in [] and {}
			for end < len(s) && (s[end] == '[' || s[end] == '{') {
				opener := s[end]
				closer := byte('}')
				if opener == '[' {
					closer = ']'
				}
				depth := 1
				end++
				for end < len(s) && depth > 0 {
					switch s[end] {
					case closer:
						depth--
					case opener:
						depth++
					}
					end++
				}
			}
			s = s[:idx] + s[end:]
		}
	}

	// Sections → markdown headings
	sectionMap := []struct {
		cmd   string
		level int
	}{
		{`\chapter`, 1},
		{`\section`, 2},
		{`\subsection`, 3},
		{`\subsubsection`, 4},
		{`\paragraph`, 5},
		{`\subparagraph`, 6},
	}
	for _, sec := range sectionMap {
		s = texReplaceCmd(s, sec.cmd, strings.Repeat("#", sec.level)+" ", "\n\n")
		// Also handle starred versions
		s = texReplaceCmd(s, sec.cmd+"*", strings.Repeat("#", sec.level)+" ", "\n\n")
	}

	// Text formatting
	s = texReplaceCmd(s, `\textbf`, "**", "**")
	s = texReplaceCmd(s, `\textit`, "*", "*")
	s = texReplaceCmd(s, `\emph`, "*", "*")
	s = texReplaceCmd(s, `\texttt`, "`", "`")
	s = texReplaceCmd(s, `\underline`, "", "")
	s = texReplaceCmd(s, `\title`, "# ", "\n\n")
	s = texReplaceCmd(s, `\author`, "*", "*\n\n")

	// Remove \maketitle, \tableofcontents, \label{...}, \ref{...}, \cite{...}
	for _, cmd := range []string{`\maketitle`, `\tableofcontents`, `\newpage`, `\clearpage`, `\noindent`} {
		s = strings.ReplaceAll(s, cmd, "")
	}
	s = texReplaceCmd(s, `\label`, "", "")
	s = texReplaceCmd(s, `\ref`, "", "")
	s = texReplaceCmd(s, `\cite`, "[", "]")
	s = texReplaceCmd(s, `\url`, "", "")
	s = texReplaceCmd(s, `\href`, "", "")
	s = texReplaceCmd(s, `\footnote`, " (", ")")

	// Environments
	// itemize / enumerate → list items
	s = strings.ReplaceAll(s, `\begin{itemize}`, "")
	s = strings.ReplaceAll(s, `\end{itemize}`, "")
	s = strings.ReplaceAll(s, `\begin{enumerate}`, "")
	s = strings.ReplaceAll(s, `\end{enumerate}`, "")
	s = strings.ReplaceAll(s, `\item`, "\n- ")

	// verbatim → code block
	s = strings.ReplaceAll(s, `\begin{verbatim}`, "\n```\n")
	s = strings.ReplaceAll(s, `\end{verbatim}`, "\n```\n")
	s = strings.ReplaceAll(s, `\begin{lstlisting}`, "\n```\n")
	s = strings.ReplaceAll(s, `\end{lstlisting}`, "\n```\n")

	// abstract
	s = strings.ReplaceAll(s, `\begin{abstract}`, "\n**Abstract:**\n")
	s = strings.ReplaceAll(s, `\end{abstract}`, "\n")

	// quote/quotation
	s = strings.ReplaceAll(s, `\begin{quote}`, "\n> ")
	s = strings.ReplaceAll(s, `\end{quote}`, "\n")
	s = strings.ReplaceAll(s, `\begin{quotation}`, "\n> ")
	s = strings.ReplaceAll(s, `\end{quotation}`, "\n")

	// Remove remaining \begin{...} and \end{...}
	for {
		idx := strings.Index(s, `\begin{`)
		if idx < 0 {
			break
		}
		end := strings.Index(s[idx:], "}")
		if end < 0 {
			break
		}
		s = s[:idx] + s[idx+end+1:]
	}
	for {
		idx := strings.Index(s, `\end{`)
		if idx < 0 {
			break
		}
		end := strings.Index(s[idx:], "}")
		if end < 0 {
			break
		}
		s = s[:idx] + s[idx+end+1:]
	}

	// Math: inline $...$ stays, display \[...\] or $$...$$ stays (useful as-is)

	// Common special characters
	s = strings.ReplaceAll(s, `\&`, "&")
	s = strings.ReplaceAll(s, `\%`, "%")
	s = strings.ReplaceAll(s, `\$`, "$")
	s = strings.ReplaceAll(s, `\#`, "#")
	s = strings.ReplaceAll(s, `\_`, "_")
	s = strings.ReplaceAll(s, `\{`, "{")
	s = strings.ReplaceAll(s, `\}`, "}")
	s = strings.ReplaceAll(s, `~`, " ")
	s = strings.ReplaceAll(s, `\\`, "\n")
	s = strings.ReplaceAll(s, `\,`, " ")
	s = strings.ReplaceAll(s, `\;`, " ")
	s = strings.ReplaceAll(s, `\quad`, "  ")
	s = strings.ReplaceAll(s, `\qquad`, "    ")
	s = strings.ReplaceAll(s, `---`, "—")
	s = strings.ReplaceAll(s, `--`, "–")

	// Clean up
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

// texReplaceCmd replaces \cmd{content} with prefix+content+suffix.
func texReplaceCmd(s, cmd, prefix, suffix string) string {
	for {
		idx := strings.Index(s, cmd)
		if idx < 0 {
			return s
		}
		// Make sure it's not a prefix of a longer command
		afterCmd := idx + len(cmd)
		if afterCmd < len(s) && s[afterCmd] != '{' && s[afterCmd] != '[' && s[afterCmd] != '*' &&
			((s[afterCmd] >= 'a' && s[afterCmd] <= 'z') || (s[afterCmd] >= 'A' && s[afterCmd] <= 'Z')) {
			// Part of a longer command name — skip past it
			s = s[:idx] + s[afterCmd:]
			continue
		}
		// Skip optional argument [...]
		pos := afterCmd
		if pos < len(s) && s[pos] == '[' {
			depth := 1
			pos++
			for pos < len(s) && depth > 0 {
				switch s[pos] {
				case '[':
					depth++
				case ']':
					depth--
				}
				pos++
			}
		}
		// Require opening brace
		if pos >= len(s) || s[pos] != '{' {
			// No argument — just remove the command
			s = s[:idx] + s[pos:]
			continue
		}
		// Find matching closing brace
		depth := 1
		start := pos + 1
		end := start
		for end < len(s) && depth > 0 {
			switch s[end] {
			case '{':
				depth++
			case '}':
				depth--
			}
			if depth > 0 {
				end++
			}
		}
		if depth != 0 {
			// Unbalanced — skip
			s = s[:idx] + s[start:]
			continue
		}
		content := s[start:end]
		s = s[:idx] + prefix + content + suffix + s[end+1:]
	}
}
