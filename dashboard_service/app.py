import os
import requests
from flask import Flask, render_template

app = Flask(__name__)
GATEWAY_URL = os.getenv("GATEWAY_URL", "http://localhost:5009")

@app.route("/")
def dashboard():
    try:
        data = requests.get(f"{GATEWAY_URL}/dashboard/data", timeout=3).json()
    except Exception as e:
        print("Errore gateway:", e)
        data = {"sensors": [], "irrigations": [], "stats": {}}
    return render_template("dashboard.html",
                           sensors=data.get("sensors", []),
                           irrigations=data.get("irrigations", []),
                           stats=data.get("stats", {}))

@app.route("/healthz")
def healthz():
    return "ok", 200

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)