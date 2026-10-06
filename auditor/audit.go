package auditor

// Audit parses data as a sequence of strict HTTP/1.1 requests. On the first
// error it stops, keeping all previously completed requests, and does not
// attempt to resynchronize.
func Audit(data []byte) *Report {
	p := &parser{data: data}
	rep := &Report{}
	for p.pos < int64(len(data)) {
		req, err := parseOne(p, len(rep.Requests))
		if err != nil {
			rep.Error = err
			return rep
		}
		rep.Requests = append(rep.Requests, *req)
	}
	return rep
}

func parseOne(p *parser, index int) (*Request, *AuditError) {
	start := p.pos
	line, err := p.readLine()
	if err != nil {
		return nil, err
	}
	method, target, rerr := parseRequestLine(line, start)
	if rerr != nil {
		return nil, rerr
	}

	var headers []header
	for {
		hOff := p.pos
		hLine, herr := p.readLine()
		if herr != nil {
			return nil, herr
		}
		if hLine == "" {
			break
		}
		h, perr := parseHeaderLine(hLine, hOff)
		if perr != nil {
			return nil, perr
		}
		headers = append(headers, h)
	}

	var clValues []header
	var teValues []header
	hostCount := 0
	for _, h := range headers {
		switch h.name {
		case "host":
			hostCount++
			if h.value == "" {
				return nil, errAt(h.off, "Host header must be non-empty")
			}
		case "content-length":
			clValues = append(clValues, h)
		case "transfer-encoding":
			teValues = append(teValues, h)
		}
	}
	if hostCount != 1 {
		return nil, errAt(start, "request must have exactly one Host header, got %d", hostCount)
	}
	if len(clValues) > 1 {
		return nil, errAt(clValues[1].off, "duplicate Content-Length headers are not allowed")
	}
	if len(clValues) > 0 && len(teValues) > 0 {
		return nil, errAt(teValues[0].off, "Content-Length with Transfer-Encoding is not allowed")
	}

	var body []byte
	switch {
	case len(teValues) > 0:
		if len(teValues) > 1 {
			return nil, errAt(teValues[1].off, "duplicate Transfer-Encoding headers are not allowed")
		}
		if teValues[0].value != "chunked" {
			return nil, errAt(teValues[0].off, "Transfer-Encoding must be exactly \"chunked\"")
		}
		b, cerr := p.readChunkedBody()
		if cerr != nil {
			return nil, cerr
		}
		body = b
	case len(clValues) > 0:
		n, cerr := parseContentLength(clValues[0].value, clValues[0].off)
		if cerr != nil {
			return nil, cerr
		}
		b, cerr := p.readFixedBody(n)
		if cerr != nil {
			return nil, cerr
		}
		body = b
	default:
		body = nil
	}

	return &Request{
		Index:       index,
		StartOffset: start,
		EndOffset:   p.pos,
		Method:      method,
		Target:      target,
		BodyLength:  int64(len(body)),
		BodySHA256:  digest(body),
	}, nil
}
