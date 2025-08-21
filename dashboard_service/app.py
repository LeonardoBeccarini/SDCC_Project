import requests
from flask import Flask, render_template

app = Flask(__name__)
GATEWAY_URL = "http://172.31.39.52:5009"
@app.route("/")
def dashboard():
    # Dati di esempio (verranno da Go più avanti)
    sensors = [
        {"id": "sensor_1", "value": 31.5, "status": "on"},
        {"id": "sensor_2", "value": 28.2, "status": "off"},
    ]

    irrigations = [
        {"sensor_id": "sensor_1", "amount": 10, "time": "07:30"},
        {"sensor_id": "sensor_2", "amount": 5, "time": "09:15"},
    ]

    # Aggiungi i dati statistici
    stats = {
        "mean": 29.85  # esempio di media
    }
    data = {}
    try:
        data = requests.get(f"{GATEWAY_URL}/dashboard/data", timeout=3).json()
    except Exception as e:
        print("Errore gateway:", e)
        data = {"sensors": [], "irrigations": [], "stats": {}}
    return render_template("dashboard.html",
                           sensors=data.get("sensors", []),
                           irrigations=data.get("irrigations", []),
                           stats=data.get("stats", {}))

    return render_template("dashboard.html", sensors=sensors, irrigations=irrigations, stats=stats)

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)