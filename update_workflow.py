import json
import uuid

def node_id():
    return str(uuid.uuid4())[:16]

# Initialize the workflow structure
workflow = {
  "nodes": [
    {
      "parameters": {
        "rule": {
          "interval": [
            {
              "field": "cronExpression",
              "expression": "0 * * * *"
            }
          ]
        }
      },
      "id": node_id(),
      "name": "Every Hour",
      "type": "n8n-nodes-base.cron",
      "typeVersion": 1,
      "position": [0, 100]
    },
    {
      "parameters": {},
      "id": node_id(),
      "name": "Manual Run",
      "type": "n8n-nodes-base.manualTrigger",
      "typeVersion": 1,
      "position": [0, 300]
    },
    {
      "parameters": {
        "jsCode": "return [ { json: { force: false } } ];"
      },
      "id": node_id(),
      "name": "Configuration",
      "type": "n8n-nodes-base.code",
      "typeVersion": 2,
      "position": [200, 200]
    },
    {
      "parameters": {
        "operation": "executeQuery",
        "query": "=SELECT id, language, script FROM episodes WHERE ({{ $json.force }} = true OR tts_at IS NULL) ORDER BY created_at DESC LIMIT 10;"
      },
      "id": node_id(),
      "name": "Postgres: Read Latest Scripts",
      "type": "n8n-nodes-base.postgres",
      "typeVersion": 2.4,
      "position": [450, 200],
      "credentials": {
        "postgres": {
          "id": "PG_CRED",
          "name": "VoiceScribe Postgres"
        }
      }
    }
  ],
  "connections": {
    "Every Hour": {
      "main": [
        [{"node": "Configuration", "type": "main", "index": 0}]
      ]
    },
    "Manual Run": {
      "main": [
        [{"node": "Configuration", "type": "main", "index": 0}]
      ]
    },
    "Configuration": {
      "main": [
        [{"node": "Postgres: Read Latest Scripts", "type": "main", "index": 0}]
      ]
    },
    "Postgres: Read Latest Scripts": {
      "main": [
        []
      ]
    }
  },
  "settings": {
    "executionOrder": "v1"
  },
  "meta": {
    "templateCredsSetupCompleted": False
  },
  "id": "VoiceScribeAudioEngine",
  "name": "VoiceScribe - Audio Render Engine",
  "active": True
}

langs = [
    {"code": "en", "name": "English"},
    {"code": "it", "name": "Italian"},
    {"code": "fr", "name": "French"},
    {"code": "bn", "name": "Bangla"}
]

for i, l in enumerate(langs):
    y_pos = 100 + (i * 250)
    
    # 1. Build Payload Node (with internal language filter)
    build_name = f"Build Payload ({l['name']})"
    workflow['nodes'].append({
      "parameters": {
        "jsCode": f"""const items = $input.all();
const targetLang = '{l['code']}';

return items.filter(item => {{
  const lang = item.json.language;
  return lang && lang.toLowerCase().substring(0,2) === targetLang.toLowerCase().substring(0,2);
}}).map(item => {{
  const json = item.json;
  const filename = `episode_{l['code']}_${{json.id}}_${{new Date().getTime()}}.wav`;
  let dialogues = [];
  try {{
    const parsed = JSON.parse(json.script);
    dialogues = parsed.dialogues || [];
  }} catch(e) {{
    const parts = json.script.split('\\n\\n');
    for (const part of parts) {{
      if (part.trim().length > 0) {{
        dialogues.push({{ speaker: 'Host', text: part.trim() }});
      }}
    }}
  }}
  return {{
    json: {{
      script: dialogues,
      language: '{l['code']}',
      filename: filename,
      episode_id: json.id
    }}
  }};
}});"""
      },
      "id": node_id(),
      "name": build_name,
      "type": "n8n-nodes-base.code",
      "typeVersion": 2,
      "position": [700, y_pos]
    })
    
    # Connect Postgres directly to Build Payload in parallel (Broadcast)
    workflow['connections']['Postgres: Read Latest Scripts']['main'][0].append({"node": build_name, "type": "main", "index": 0})
    
    # 2. TTS Request Node
    tts_name = f"Generate TTS ({l['name']})"
    workflow['nodes'].append({
      "parameters": {
        "method": "POST",
        "url": "http://webhook-app:8080/tts",
        "sendBody": True,
        "specifyBody": "json",
        "jsonBody": "={{ $json }}",
        "options": {
          "retryOnFail": True,
          "maxTries": 3,
          "waitBetweenTries": 5000
        }
      },
      "id": node_id(),
      "name": tts_name,
      "type": "n8n-nodes-base.httpRequest",
      "typeVersion": 4.2,
      "position": [1000, y_pos]
    })
    
    # 3. Update DB Node
    db_name = f"Update Episode ({l['name']})"
    workflow['nodes'].append({
      "parameters": {
        "operation": "executeQuery",
        "query": f"=UPDATE episodes SET tts_at = NOW(), mp3_url = '{{{{ $json.url }}}}' WHERE id = {{{{ $('{build_name}').item.json.episode_id }}}};"
      },
      "id": node_id(),
      "name": db_name,
      "type": "n8n-nodes-base.postgres",
      "typeVersion": 2.4,
      "position": [1250, y_pos],
      "credentials": {
        "postgres": {
          "id": "PG_CRED",
          "name": "VoiceScribe Postgres"
        }
      }
    })
    
    # Final connections for the branch
    workflow['connections'][build_name] = {"main": [[{"node": tts_name, "type": "main", "index": 0}]]}
    workflow['connections'][tts_name] = {"main": [[{"node": db_name, "type": "main", "index": 0}]]}

with open('n8n/workflows/tts-only-pipeline.json', 'w') as f:
    json.dump(workflow, f, indent=2, ensure_ascii=False)

print("Done")
