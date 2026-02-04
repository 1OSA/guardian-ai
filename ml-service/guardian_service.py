import pickle
from contextlib import asynccontextmanager

import numpy as np
import tensorflow as tf
import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from tensorflow.keras.preprocessing.sequence import pad_sequences

# --- CONFIG ---
MAX_LEN = 75
MODEL_PATH = "guardian_model.h5"
TOKENIZER_PATH = "tokenizer.pickle"

# Global variables
ai_resources = {}


# --- LIFESPAN MANAGER (Replaces the deprecated @app.on_event) ---
@asynccontextmanager
async def lifespan(app: FastAPI):
    # Load resources on startup
    print("[*] Loading Guardian AI Model...")
    try:
        ai_resources["model"] = tf.keras.models.load_model(MODEL_PATH)
        print("[+] Model loaded successfully.")
    except Exception as e:
        print(f"[-] CRITICAL ERROR: Could not load model. {e}")

    print("[*] Loading Tokenizer...")
    try:
        with open(TOKENIZER_PATH, "rb") as f:
            ai_resources["tokenizer"] = pickle.load(f)
        print("[+] Tokenizer loaded successfully.")
    except Exception as e:
        print(f"[-] CRITICAL ERROR: Could not load tokenizer. {e}")

    yield
    # Clean up on shutdown (if needed)
    ai_resources.clear()


app = FastAPI(title="Guardian AI DNS Filter", lifespan=lifespan)


# --- API CONTRACT ---
class DomainRequest(BaseModel):
    domain: str


# --- PREDICTION ENDPOINT ---
@app.post("/predict")
async def predict_domain(request: DomainRequest):
    if "model" not in ai_resources or "tokenizer" not in ai_resources:
        raise HTTPException(status_code=500, detail="AI Model not loaded")

    domain = request.domain.lower().strip()

    # 1. Preprocess
    seq = ai_resources["tokenizer"].texts_to_sequences([domain])
    padded = pad_sequences(seq, maxlen=MAX_LEN, padding="post")

    # 2. Inference
    prediction = ai_resources["model"].predict(padded, verbose=0)

    # 3. Interpret Results
    class_idx = np.argmax(prediction[0])
    confidence = float(prediction[0][class_idx])  # Convert numpy float to python float

    labels = {0: "Safe", 1: "DGA", 2: "Phishing"}

    # Safety Logic: If confidence is weak, fail open (Safe)
    if confidence < 0.60:
        class_idx = 0
        verdict = "Safe (Low Confidence)"
    else:
        verdict = labels.get(class_idx, "Unknown")

    # --- CRITICAL FIX: CONVERT NUMPY TO PYTHON TYPES ---
    # FastAPI crashes if you send numpy.bool_ or numpy.float32
    is_blocked = bool(class_idx > 0)

    return {
        "domain": domain,
        "result": verdict,
        "block": is_blocked,  # explicit python bool
        "confidence": round(confidence * 100, 2),
        "probabilities": {
            "safe": float(prediction[0][0]),  # explicit python float
            "dga": float(prediction[0][1]),  # explicit python float
            "phishing": float(prediction[0][2]),  # explicit python float
        },
    }


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=5000)
