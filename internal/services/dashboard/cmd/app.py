import os, requests
from flask import Flask, render_template
app = Flask(__name__)
GATEWAY_URL = os.getenv("GATEWAY_URL", "http://gateway:5009")

@app.route("/")
def home():
    try:
        r = requests.get(f"{GATEWAY_URL}/dashboard/data", timeout=3)
        data = r.json() if r.ok else {}
    except Exception:
        data = {}
    sensors     = data.get("sensors") or []
    irrigations = data.get("irrigations") or []
    stats       = data.get("stats") or {}
    return render_template("dashboard.html",
                           sensors=sensors, irrigations=irrigations, stats=stats)

@app.route("/healthz")
def healthz():
    return "ok", 200

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)
