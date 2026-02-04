import pickle
import time
from concurrent import futures

import grpc

# Import the generated code
import guardian_pb2
import guardian_pb2_grpc

#
import numpy as np
import tensorflow as tf
from tensorflow.keras.preprocessing.sequence import pad_sequences

# --- CONFIG ---
MAX_LEN = 75
MODEL_PATH = "guardian_model.h5"
TOKENIZER_PATH = "tokenizer.pickle"


class GuardianService(guardian_pb2_grpc.GuardianAIServicer):
    def __init__(self):
        print("[*] Loading Guardian AI Model...")
        self.model = tf.keras.models.load_model(MODEL_PATH)

        print("[*] Loading Tokenizer...")
        with open(TOKENIZER_PATH, "rb") as f:
            self.tokenizer = pickle.load(f)
        print("[+] AI Ready to serve.")

    def PredictDomain(self, request, context):
        domain = request.domain.lower().strip()

        # 1. Preprocess
        seq = self.tokenizer.texts_to_sequences([domain])
        padded = pad_sequences(seq, maxlen=MAX_LEN, padding="post")

        # 2. Inference
        prediction = self.model.predict(padded, verbose=0)

        # 3. Interpret
        class_idx = np.argmax(prediction[0])
        confidence = float(prediction[0][class_idx])

        labels = {0: "Safe", 1: "DGA", 2: "Phishing"}

        # Safety Fail-Open Logic
        if confidence < 0.60:
            class_idx = 0
            verdict = "Safe (Low Confidence)"
        else:
            verdict = labels.get(class_idx, "Unknown")

        is_malicious = bool(class_idx > 0)

        # 4. Return gRPC Response
        return guardian_pb2.PredictionResponse(
            is_malicious=is_malicious, confidence_score=confidence, category=verdict
        )


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    guardian_pb2_grpc.add_GuardianAIServicer_to_server(GuardianService(), server)

    # Listen on port 50051
    server.add_insecure_port("[::]:50051")
    print("[*] gRPC Server running on port 50051...")
    server.start()
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        server.stop(0)


if __name__ == "__main__":
    serve()
