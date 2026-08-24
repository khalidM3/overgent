# Gate E — intelligence evaluation seed

This disposable Go program evaluates the versioned synthetic corpus without a
network or model provider. It records three candidate-retrieval baselines:

- exact path intersection;
- token Jaccard similarity; and
- cosine similarity over explicitly synthetic concept vectors.

The vectors are fixture data, not a claim about production embedding quality.
They make the public evaluation mechanics and expected routing labels executable
before a provider is selected. Proactive semantic notifications remain disabled.

Run:

```bash
go test ./...
go run . corpus.json
```
