#!/usr/bin/env python3
"""Apply per-region freshness + rank-aware update tracking fix to news-pipeline-v4.json.

Edits, in order:
  1. Add 4 "Set Region" Code nodes between each trigger and Source list.
  2. Tag Source list output with region propagated from the Set Region node.
  3. Rewrite pg-check-existing to LEFT JOIN news_item_airings filtered by current region.
  4. Rewrite filter-uncached so top-rank URLs are eligible for re-scrape.
  5. Rewrite pg-save-raw to compute body_hash and stamp body_updated_at on real change.
  6. Rewrite pg-read-raw to return only this-run fingerprints with region-scoped flags.
  7. Extend build-editor-payload to pass is_repeat / has_update / first_aired_at.
  8. Extend filter-selected to attach rank = idx + 1.
  9. Flip summary-cached-check to a needs_resummary boolean.
 10. Extend Build Summary Payload to use "Update on …" framing for repeats with updates.
 11. Add Postgres: Persist airings node after Save (English).
 12. Rewire connections: triggers → Set Region (X) → Source list,
                         Save (English) → Persist airings → Regenerate RSS Feeds.
"""
from __future__ import annotations
import json
import sys
from pathlib import Path

WORKFLOW = Path('/home/fauzul/code/hackathon-lab/n8n/workflows/news-pipeline-v4.json')

REGION_BY_TRIGGER = {
    # Manual Run maps to 'london' so local testing exercises a valid region
    # (the schema CHECK constraint only permits london/rome_paris/dhaka).
    'Manual Run': ('Set Region (Manual)', 'set-region-manual', 'london', [-300, 100]),
    'London Schedule (EN)': ('Set Region (London)', 'set-region-london', 'london', [-300, 200]),
    'Rome & Paris Schedule (IT/FR)': ('Set Region (Rome/Paris)', 'set-region-rome', 'rome_paris', [-300, 350]),
    'Dhaka Schedule (BN)': ('Set Region (Dhaka)', 'set-region-dhaka', 'dhaka', [-300, 650]),
}

REGION_EXPR = "{{ $('Source list').first().json.region }}"

# ---------- node patches (keyed by node id) ----------

FILTER_URLS_JS = """const data = $input.first().json;
const source = $('Source list').first().json.source;
const links = (data.data && data.data.links) ? data.data.links : [];
const MAX_ARTICLES = 10;  // 10 per source x 3 sources = 30 total candidates

const hash = (s) => {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return (h >>> 0).toString(16);
};

return links.slice(0, MAX_ARTICLES).map(url => ({
  json: { url, source_name: source, fingerprint: hash(url) }
}));"""

BARRIER_JS = """// Barrier: emit input items exactly once per workflow execution.
// Source list outputs 3 items (TechCrunch / VentureBeat / MIT Tech Review).
// That fan-out causes the editor -> save -> persist-airings chain to fire
// 2-3 times per execution, producing duplicate episodes. This barrier
// collapses those repeated firings to a single forward pass.
const staticData = $getWorkflowStaticData('node');
staticData.seenExecs = staticData.seenExecs || {};
if (staticData.seenExecs[$execution.id]) {
  return [];
}
staticData.seenExecs[$execution.id] = Date.now();
// Keep static data bounded: drop entries older than 1 day.
const cutoff = Date.now() - 86400000;
for (const k of Object.keys(staticData.seenExecs)) {
  if (staticData.seenExecs[k] < cutoff) delete staticData.seenExecs[k];
}
return $input.all();"""

PG_CHECK_EXISTING_QUERY = (
    "=SELECT n.fingerprint, n.body_hash, n.body_updated_at, "
    "a.last_rank, a.last_aired_at, a.first_aired_at "
    "FROM news_items n "
    "LEFT JOIN news_item_airings a "
    "ON a.fingerprint = n.fingerprint AND a.region = '" + REGION_EXPR + "' "
    "WHERE n.fingerprint IN ("
    "{{ $input.all().map(i => \"'\" + i.json.fingerprint + \"'\").join(',') || \"''\" }}"
    ");"
)

FILTER_UNCACHED_JS = """const allUrls = $('Filter article URLs').all();
const existingRows = $('Postgres: Check existing').all();
const existingByFp = new Map(existingRows.map(r => [r.json.fingerprint, r.json]));

// Top-rank articles in THIS region are eligible for re-scrape so we can
// detect body updates. Cooldown prevents hammering Firecrawl on the same URL.
const TOP_RANK = 3;
const COOLDOWN_HOURS = 2;

return allUrls.filter(u => {
  const existing = existingByFp.get(u.json.fingerprint);
  if (!existing) return true;
  if (existing.last_rank == null || existing.last_rank > TOP_RANK) return false;
  const lastHit = existing.last_aired_at ? new Date(existing.last_aired_at) : null;
  if (lastHit && (Date.now() - lastHit.getTime()) < COOLDOWN_HOURS * 3600 * 1000) return false;
  return true;
});"""

PG_SAVE_RAW_QUERY = (
    "=INSERT INTO news_items "
    "(fingerprint, headline, body, source_name, article_url, language, scraped_at, body_hash, body_updated_at) "
    "VALUES ("
    "'{{ $node[\"Per article\"].json.fingerprint }}', "
    "'{{ $json.data.metadata.title.replace(/'/g, \"''\") }}', "
    "'{{ $json.data.markdown.replace(/'/g, \"''\") }}', "
    "'{{ $node[\"Per article\"].json.source_name }}', "
    "'{{ $node[\"Per article\"].json.url }}', "
    "'en', NOW(), "
    "md5('{{ $json.data.markdown.replace(/'/g, \"''\") }}'), "
    "NOW()"
    ") ON CONFLICT (fingerprint) DO UPDATE SET "
    "scraped_at = NOW(), "
    "headline = EXCLUDED.headline, "
    "body = EXCLUDED.body, "
    "body_hash = EXCLUDED.body_hash, "
    "body_updated_at = CASE "
    "WHEN news_items.body_hash IS DISTINCT FROM EXCLUDED.body_hash THEN NOW() "
    "ELSE news_items.body_updated_at "
    "END;"
)

PG_READ_RAW_QUERY = (
    "=SELECT n.fingerprint, n.headline, n.body, n.source_name, n.llm_summary, "
    "n.body_updated_at, n.summarized_at, "
    "a.last_rank, a.last_aired_at, a.first_aired_at, "
    "(a.fingerprint IS NOT NULL) AS is_repeat, "
    "(a.fingerprint IS NOT NULL AND n.body_updated_at > a.last_aired_at) AS has_update, "
    "(n.llm_summary IS NULL OR n.llm_summary = '' OR n.summarized_at IS NULL "
    " OR n.body_updated_at > n.summarized_at) AS needs_resummary "
    "FROM news_items n "
    "LEFT JOIN news_item_airings a "
    "ON a.fingerprint = n.fingerprint AND a.region = '" + REGION_EXPR + "' "
    "WHERE n.fingerprint IN ("
    "{{ $('Filter article URLs').all().map(i => \"'\" + i.json.fingerprint + \"'\").join(',') || \"''\" }}"
    ") "
    "AND (a.fingerprint IS NULL OR n.body_updated_at > a.last_aired_at) "
    "ORDER BY (a.fingerprint IS NULL) DESC, a.last_rank ASC NULLS LAST, n.scraped_at DESC;"
)

BUILD_EDITOR_PAYLOAD_JS = """const MAX_ARTICLES = 10;
const MIN_ARTICLES = 7;
const items = $input.all();

const systemPrompt = [
  `You are an autonomous AI news editor. Your task is to select the BEST articles for a daily briefing targeting busy tech professionals and business leaders.`,
  ``,
  `CRITICAL RULES — YOU MUST FOLLOW THESE EXACTLY:`,
  `1. You MUST select a MINIMUM of ${MIN_ARTICLES} articles and a MAXIMUM of ${MAX_ARTICLES} articles.`,
  `2. Selecting fewer than ${MIN_ARTICLES} or more than ${MAX_ARTICLES} is a VIOLATION. Do not do it.`,
  `3. Rank by: business impact, novelty, and relevance to tech industry.`,
  `4. Exclude: paywalled articles, listicles, press releases, and opinion pieces with no news value.`,
  `5. PREFER articles where is_repeat is false (fresh in this region). Only include articles where is_repeat is true if has_update is true — those are genuine updates worth re-airing. NEVER select an article where is_repeat is true and has_update is false; those are stale repeats.`,
  `6. Output ONLY valid JSON with exactly two keys:`,
  `   - "selected_fingerprints": an array of EXACTLY ${MIN_ARTICLES}–${MAX_ARTICLES} fingerprint strings`,
  `   - "rationale": one short sentence explaining the selection`
].join('\\n');

const userContent = [
  `Here are ${items.length} articles to evaluate. Select between ${MIN_ARTICLES} and ${MAX_ARTICLES} of them.`,
  ``,
  JSON.stringify(items.map(i => ({
    fingerprint: i.json.fingerprint,
    headline: i.json.headline,
    source: i.json.source_name,
    is_repeat: !!i.json.is_repeat,
    has_update: !!i.json.has_update,
    first_aired_at: i.json.first_aired_at || null
  }))),
  ``,
  `REMINDER: Your response MUST contain between ${MIN_ARTICLES} and ${MAX_ARTICLES} fingerprints in selected_fingerprints. Not more, not fewer.`
].join('\\n');

const payload = {
  messages: [
    { role: 'system', content: systemPrompt },
    { role: 'user', content: userContent }
  ],
  temperature: 0,
  response_format: { type: 'json_object' }
};
return [{ json: { llm_payload: payload, max_articles: MAX_ARTICLES, min_articles: MIN_ARTICLES } }];"""

FILTER_SELECTED_JS = """const input = $input.first().json;
let rawContent = input.choices[0].message.content || '';
rawContent = rawContent.replace(/^```(?:json)?\\n/i, '').replace(/\\n```$/i, '').trim();

let decision;
try {
  decision = JSON.parse(rawContent);
} catch (e) {
  throw new Error(`Failed to parse LLM response as JSON. Raw content: ${JSON.stringify(rawContent)}`);
}
let fps = decision.selected_fingerprints;
if (!Array.isArray(fps)) fps = [];

const maxArticles = $('Build Editor Payload').first().json.max_articles || 10;
const orderedFps = fps.slice(0, maxArticles);
const rankByFp = new Map(orderedFps.map((fp, idx) => [fp, idx + 1]));

// Preserve the editor's selection order so rank tracks LLM ranking.
const allItems = $('Postgres: Read raw articles').all();
const byFp = new Map(allItems.map(i => [i.json.fingerprint, i.json]));
const selected = orderedFps
  .map(fp => byFp.get(fp))
  .filter(Boolean);

const targetLength = Math.max(30, Math.floor(350 / Math.max(1, selected.length)));
return selected.map(rawItem => ({
  json: {
    ...rawItem,
    rank: rankByFp.get(rawItem.fingerprint),
    target_length: targetLength
  }
}));"""

BUILD_SUMMARY_PAYLOAD_JS = """const item = $json;

let firstAiredLabel = null;
if (item.first_aired_at) {
  try {
    firstAiredLabel = new Date(item.first_aired_at).toISOString().slice(0, 10);
  } catch (e) {
    firstAiredLabel = null;
  }
}

let systemPrompt;
if (item.is_repeat && item.has_update) {
  systemPrompt = `Summarize this article in approximately ${item.target_length} words for a podcast anchor targeting business leaders. This story has been aired before in this region${firstAiredLabel ? ' (first reported ' + firstAiredLabel + ')' : ''}; the body has been updated since then. Open with "Update on [topic]${firstAiredLabel ? ' (first reported ' + firstAiredLabel + ')' : ''}:" and focus on what is genuinely new. Keep it concise, action-oriented, business-impact focused. Spoken prose, professional tone.`;
} else if (item.is_repeat && !item.has_update) {
  // Safety net — editor should not have selected this. Drop the item so
  // it never reaches Azure OpenAI with a missing payload.
  return [];
} else {
  systemPrompt = `Summarize this article in exactly ${item.target_length} words for a podcast anchor targeting business leaders. Keep it concise, action-oriented, and focus on the business impact. Spoken prose, professional tone.`;
}

const payload = {
  messages: [
    { role: 'system', content: systemPrompt },
    { role: 'user', content: `Headline: ${item.headline}\\n\\nContent: ${item.body}` }
  ],
  temperature: 0.7
};
return [{ json: { ...item, llm_payload: payload } }];"""

SUMMARY_CACHED_CONDITIONS = {
    "options": {
        "caseSensitive": True,
        "leftValue": "",
        "typeValidation": "strict"
    },
    "conditions": [
        {
            "id": "summary-needs-resummary-check",
            "leftValue": "={{ $json.needs_resummary }}",
            "rightValue": True,
            "operator": {
                "type": "boolean",
                "operation": "equals"
            }
        }
    ],
    "combinator": "and"
}

PERSIST_AIRINGS_QUERY = (
    # GROUP BY v.fp collapses duplicate (fingerprint, rank) pairs that can
    # appear when the editor LLM picks the same article twice or when an
    # upstream node aggregates across multiple firings. Without this, the
    # INSERT trips PostgreSQL's "ON CONFLICT DO UPDATE command cannot affect
    # row a second time" rule. MIN(r) keeps the best (lowest) rank when
    # duplicates exist.
    "=INSERT INTO news_item_airings "
    "(fingerprint, region, last_rank, last_aired_at, first_aired_at, aired_count) "
    "SELECT v.fp, '" + REGION_EXPR + "', MIN(v.r) AS r, NOW(), NOW(), 1 "
    "FROM (VALUES "
    "{{ $('Filter Selected').all().map(i => `('${i.json.fingerprint}', ${i.json.rank})`).join(',') }}"
    ") AS v(fp, r) "
    "GROUP BY v.fp "
    "ON CONFLICT (fingerprint, region) DO UPDATE SET "
    "last_rank = EXCLUDED.last_rank, "
    "last_aired_at = NOW(), "
    "aired_count = news_item_airings.aired_count + 1;"
)


def find_node(nodes, *, node_id=None, name=None):
    for n in nodes:
        if node_id is not None and n.get('id') == node_id:
            return n
        if name is not None and n.get('name') == name:
            return n
    raise KeyError(f"node id={node_id!r} name={name!r} not found")


def main() -> int:
    wf = json.loads(WORKFLOW.read_text(encoding='utf-8'))
    nodes = wf['nodes']
    conns = wf['connections']

    # --- 1 & 12a. Add Set Region nodes; rewire triggers ---
    for trigger_name, (sr_name, sr_id, region, pos) in REGION_BY_TRIGGER.items():
        # Manual Run reads REGION env var (default 'london') so a single
        # `docker compose exec -e REGION=<region> n8n n8n execute --id …`
        # can simulate any region's cron fire. Schedule triggers stay
        # hardcoded to their region.
        if trigger_name == 'Manual Run':
            js = (
                "const region = ($env && $env.REGION) || '" + region + "';\n"
                "return [{ json: { region } }];"
            )
        else:
            js = f"return [{{ json: {{ region: '{region}' }} }}];"
        nodes.append({
            "parameters": {
                "jsCode": js
            },
            "id": sr_id,
            "name": sr_name,
            "type": "n8n-nodes-base.code",
            "typeVersion": 2,
            "position": pos
        })
        # Rewrite trigger's outgoing connection: trigger → Set Region (X)
        conns[trigger_name] = {
            "main": [[{"node": sr_name, "type": "main", "index": 0}]]
        }
        # Add Set Region → Source list
        conns[sr_name] = {
            "main": [[{"node": "Source list", "type": "main", "index": 0}]]
        }

    # --- 2. Source list: propagate region from incoming Set Region item ---
    src_list = find_node(nodes, node_id='src-list')
    src_list['parameters']['jsCode'] = (
        "const region = ($input.first() && $input.first().json && $input.first().json.region) || 'manual';\n"
        "return [\n"
        "  { json: { url: 'https://techcrunch.com', source: 'TechCrunch', region } },\n"
        "  { json: { url: 'https://venturebeat.com', source: 'VentureBeat', region } },\n"
        "  { json: { url: 'https://www.technologyreview.com', source: 'MIT Tech Review', region } }\n"
        "];"
    )

    # --- (new) Reduce per-source scrape from 25 to 10 articles ---
    find_node(nodes, node_id='filter-urls')['parameters']['jsCode'] = FILTER_URLS_JS

    # --- (new) Insert "Barrier: once per exec" between Per article (done) and
    # Postgres: Get next episode number. Collapses Source-list fan-out so the
    # editor runs exactly once per execution.
    if not any(n.get('id') == 'barrier-once-per-exec' for n in nodes):
        nodes.append({
            "parameters": {"jsCode": BARRIER_JS},
            "id": "barrier-once-per-exec",
            "name": "Barrier: once per exec",
            "type": "n8n-nodes-base.code",
            "typeVersion": 2,
            "position": [950, 350]
        })
    # Rewire Per article output[0] to barrier, and barrier to Get next episode num.
    conns["Per article"]["main"][0] = [
        {"node": "Barrier: once per exec", "type": "main", "index": 0}
    ]
    conns["Barrier: once per exec"] = {
        "main": [[
            {"node": "Postgres: Get next episode number", "type": "main", "index": 0}
        ]]
    }

    # --- 3. pg-check-existing query ---
    find_node(nodes, node_id='pg-check-existing')['parameters']['query'] = PG_CHECK_EXISTING_QUERY

    # --- 4. filter-uncached jsCode ---
    find_node(nodes, node_id='filter-uncached')['parameters']['jsCode'] = FILTER_UNCACHED_JS

    # --- 5. pg-save-raw query ---
    find_node(nodes, node_id='pg-save-raw')['parameters']['query'] = PG_SAVE_RAW_QUERY

    # --- 6. pg-read-raw query (and switch to expression form, indicated by '=' prefix) ---
    find_node(nodes, node_id='pg-read-raw')['parameters']['query'] = PG_READ_RAW_QUERY

    # --- 7. build-editor-payload jsCode ---
    find_node(nodes, node_id='build-editor-payload')['parameters']['jsCode'] = BUILD_EDITOR_PAYLOAD_JS

    # --- 8. filter-selected jsCode (attach rank) ---
    find_node(nodes, node_id='filter-selected')['parameters']['jsCode'] = FILTER_SELECTED_JS

    # --- 9. summary-cached-check conditions (flip to needs_resummary) ---
    find_node(nodes, node_id='summary-cached-check')['parameters']['conditions'] = SUMMARY_CACHED_CONDITIONS

    # The "Summary Cached?" IF used to be: TRUE branch (llm_summary empty) → Build Summary Payload,
    # FALSE branch (llm_summary non-empty) → Per article for summary (skip).
    # Now the check is needs_resummary == true. Semantics: TRUE → re-summarize, FALSE → skip.
    # Connections already wire TRUE → Build Summary Payload, FALSE → Per article for summary — matches.

    # --- 10. Build Summary Payload jsCode ---
    find_node(nodes, node_id='summary-payload')['parameters']['jsCode'] = BUILD_SUMMARY_PAYLOAD_JS

    # --- 11. Postgres: Persist airings node ---
    persist_node = {
        "parameters": {
            "operation": "executeQuery",
            "query": PERSIST_AIRINGS_QUERY
        },
        "id": "pg-persist-airings",
        "name": "Postgres: Persist airings",
        "type": "n8n-nodes-base.postgres",
        "typeVersion": 2.4,
        "position": [2750, 400],
        "credentials": {
            "postgres": {
                "id": "PG_CRED",
                "name": "VoiceScribe Postgres"
            }
        }
    }
    nodes.append(persist_node)

    # --- 12b. Save (English) → Persist airings → Regenerate RSS Feeds ---
    # Replace Save (English)'s downstream to point at Persist airings, then add
    # Persist airings → Regenerate RSS Feeds.
    conns["Save (English)"] = {
        "main": [[{"node": "Postgres: Persist airings", "type": "main", "index": 0}]]
    }
    conns["Postgres: Persist airings"] = {
        "main": [[{"node": "Regenerate RSS Feeds", "type": "main", "index": 0}]]
    }

    WORKFLOW.write_text(json.dumps(wf, indent=2, ensure_ascii=False) + '\n', encoding='utf-8')
    print("ok")
    return 0


if __name__ == '__main__':
    sys.exit(main())
