#!/bin/bash
set -u
curl -s -o /dev/null -w 'visualizer=%{http_code} type=%{content_type}\n' http://localhost:18080/visualizer
curl -s -o /dev/null -w 'episodes_api=%{http_code} type=%{content_type}\n' http://localhost:18080/episodes
curl -s http://localhost:18080/episodes > /tmp/eps.json
python3 - <<'PY'
import json
d = json.load(open("/tmp/eps.json"))
print(f"episodes: {len(d)}")
for e in d[:8]:
    arts = e.get("articles") or []
    print(f"  #{e['episode_number']:>2} {e['language']} {e['created_at'][:19]}  {len(arts)} articles")
PY
