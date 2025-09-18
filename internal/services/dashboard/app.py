import os, requests
from flask import Flask, render_template

app = Flask(__name__)

# Gateway base URL (es. http://gateway:5009 o https://<env>.elasticbeanstalk.com)
GATEWAY_URL = os.getenv("GATEWAY_URL", "http://gateway:5009").rstrip("/")

DASHBOARD_DATA_PATH = "/dashboard/data"

def fetch_data():
    url = f"{GATEWAY_URL}{DASHBOARD_DATA_PATH}"
    try:
        r = requests.get(url, timeout=3)
        r.raise_for_status()
        payload = r.json()
    except Exception as e:
        app.logger.exception("fetch_data failed for %s: %s", url, e)
        return {}

    # Normalizza il payload in un dict con chiavi previste dal template
    if not payload:
        return {}

    if isinstance(payload, dict):
        return payload

    if "irrigations" in payload:
        payload["irrigations"] = sorted(payload["irrigations"], key=lambda x: x['time'])
        return {"sensors": payload}

    app.logger.warning("Unexpected payload type: %r", type(payload))
    return {}

@app.route("/")
def home():
    data = fetch_data() or {}
    sensors     = data.get("sensors", [])
    irrigations = data.get("irrigations", [])
    stats       = data.get("stats", {})
    return render_template("dashboard.html",
                           sensors=sensors, irrigations=irrigations, stats=stats)

@app.route("/health")
def health():
    return {"status": "ok"}, 200

@app.route("/healthz")
def healthz():
    return "ok", 200

if __name__ == "__main__":
    # Porta 8080 per compatibilità con Elastic Beanstalk (Docker platform)
    app.run(host="0.0.0.0", port=8080)
