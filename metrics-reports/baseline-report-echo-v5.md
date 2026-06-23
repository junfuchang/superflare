# SuperFlare Baseline Metrics (Echo V5)

This report is the current baseline after switching the web stack to Echo v5.

- generated_at: 2026-02-16T08:05:30Z
- requests: 2000
- concurrency: 8
- warmup: 100

| scenario | endpoint | QPS | P50 | P95 | P99 | allocs/op | B/op |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| home | GET / | 9225.43 | 851us | 1.372ms | 2.001ms | 2602.23 | 228834.02 |
| home-search | POST / | 9133.92 | 866us | 1.451ms | 1.911ms | 2631.38 | 233986.88 |
| bookmarks | GET /bookmarks | 11000.46 | 684us | 1.313ms | 1.859ms | 2094.21 | 186453.62 |
| applications | GET /applications | 17278.75 | 412us | 893us | 1.179ms | 859.72 | 97003.07 |
| redir-url | GET /redir/url?go=aHR0cHM6Ly9saW5rLmV4YW1wbGUuY29t | 30330.47 | 204us | 596us | 1.048ms | 450.91 | 31038.76 |
