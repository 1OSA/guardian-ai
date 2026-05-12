import json
import logging
import os
import sys
from concurrent import futures

import grpc
import numpy as np

# Try to import ONNX runtime, fall back to dummy if not available
try:
    import onnxruntime as ort

    HAS_ONNX = True
except ImportError:
    HAS_ONNX = False

# Import generated gRPC code
import guardian_pb2
import guardian_pb2_grpc

# Setup logging
logging.basicConfig(
    level=logging.INFO,
    format="[ml] %(asctime)s %(levelname)s: %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
logger = logging.getLogger(__name__)


class GuardianAI(guardian_pb2_grpc.GuardianAIServicer):
    # Category index mapping
    # IMPORTANT: keep this mapping in sync with how the model was trained.
    # The training script used indices: 0=Safe, 1=DGA, 2=Phishing (see train_model.py labels list).
    # Map numeric indices to category names returned to the server.
    CATEGORY_MAP = {
        0: "safe",
        1: "dga",
        2: "phishing",
        "safe": "safe",
        "phishing": "phishing",
        "dga": "dga",
        "malware": "malware",
        "other": "other",
    }

    def __init__(self, model_path, tokenizer_path):
        self.model_path = model_path
        self.tokenizer_path = tokenizer_path
        self.model = None
        self.tokenizer = None
        self.load_model()
        self.load_tokenizer()

    def load_model(self):
        """Load ONNX model"""
        if not HAS_ONNX:
            logger.warning("ONNX Runtime not available, using dummy predictions")
            return

        if not os.path.exists(self.model_path):
            logger.error(f"Model file not found: {self.model_path}")
            return

        try:
            self.model = ort.InferenceSession(self.model_path)
            logger.info(f"Loaded model from {self.model_path}")
        except Exception as e:
            logger.error(f"Failed to load model: {e}")

    def load_tokenizer(self):
        """Load tokenizer from JSON"""
        if not os.path.exists(self.tokenizer_path):
            logger.error(f"Tokenizer file not found: {self.tokenizer_path}")
            return

        try:
            with open(self.tokenizer_path, "r", encoding="utf-8") as f:
                self.tokenizer = json.load(f)
            logger.info(f"Loaded tokenizer from {self.tokenizer_path}")
        except Exception as e:
            logger.error(f"Failed to load tokenizer: {e}")

    def tokenize(self, text):
        """Tokenize domain exactly matching Keras Tokenizer texts_to_sequences"""
        indices = []
        if not self.tokenizer:
            logger.warning("Tokenizer not loaded, using fallback zeros")
            return [0.0] * 75

        word_index = self.tokenizer.get("word_index", {})
        oov_index = self.tokenizer.get("oov_index", 0)

        # Lowercase if the tokenizer was configured to do so
        if self.tokenizer.get("lower", True):
            text = text.lower()

        for char in text[:75]:
            idx = word_index.get(char, oov_index)
            indices.append(float(idx))

        # Pad to 75 (padding='post')
        while len(indices) < 75:
            indices.append(0.0)

        return indices[:75]

    def predict(self, domain):
        """Run inference on domain"""
        # Preprocess: tokenize domain
        tokens = self.tokenize(domain.lower())
        input_data = np.array([tokens], dtype=np.float32)

        if self.model is None:
            # Dummy prediction when model not available
            logger.warning(f"Model not loaded, returning dummy prediction for {domain}")
            return {"is_malicious": False, "confidence": 0.5, "category": "unknown"}

        try:
            # Run inference
            input_name = self.model.get_inputs()[0].name
            output_names = [o.name for o in self.model.get_outputs()]

            outputs = self.model.run(output_names, {input_name: input_data})

            # Model outputs softmax probabilities: [phishing_prob, malware_prob, dga_prob]
            # Extract probabilities from first (and usually only) output
            if len(outputs) > 0:
                probs = np.asarray(outputs[0]).flatten()

                if len(probs) >= 3:
                    # Get the category with highest probability
                    category_idx = int(np.argmax(probs))
                    confidence = float(probs[category_idx])

                    # Map category index to name
                    category = self.CATEGORY_MAP.get(category_idx, "unknown")

                    # Safe class should never be treated as malicious, regardless of confidence
                    if category == "safe":
                        is_malicious = False
                    else:
                        is_malicious = confidence > 0.5

                    # Log raw probabilities for debugging (helps detect label-order mismatches)
                    logger.debug(
                        f"Model probs: {probs.tolist()} -> idx={category_idx} ({category}) confidence={confidence:.4f}"
                    )
                else:
                    # Fallback if fewer than 3 probabilities
                    is_malicious = bool(float(probs[0]) > 0.5)
                    confidence = float(probs[0])
                    category = "unknown"
            else:
                is_malicious = False
                confidence = 0.5
                category = "unknown"

            return {
                "is_malicious": is_malicious,
                "confidence": confidence,
                "category": category,
            }
        except Exception as e:
            logger.error(f"Inference failed: {e}")
            return {"is_malicious": False, "confidence": 0.5, "category": "error"}

    def PredictDomain(self, request, context):
        """gRPC RPC: predict if domain is malicious"""
        domain = request.domain

        try:
            result = self.predict(domain)

            return guardian_pb2.PredictionResponse(
                is_malicious=result["is_malicious"],
                confidence_score=result["confidence"],
                category=result["category"],
            )
        except Exception as e:
            logger.error(f"PredictDomain failed for {domain}: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f"Prediction failed: {e}")
            return guardian_pb2.PredictionResponse()


def serve():
    """Start gRPC server"""
    # Get script directory
    script_dir = os.path.dirname(os.path.abspath(__file__))

    # Model and tokenizer paths
    model_path = os.path.join(script_dir, "guardian_model.onnx")
    tokenizer_path = os.path.join(script_dir, "tokenizer.json")

    logger.info("Starting Guardian AI ML Service")
    logger.info(f"Model path: {model_path}")
    logger.info(f"Tokenizer path: {tokenizer_path}")

    # Create server
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))

    # Add servicer
    servicer = GuardianAI(model_path, tokenizer_path)
    guardian_pb2_grpc.add_GuardianAIServicer_to_server(servicer, server)

    # Bind to port
    server.add_insecure_port("127.0.0.1:50051")
    server.start()

    logger.info("ML Service listening on 127.0.0.1:50051")

    try:
        # Keep server running
        while True:
            import time

            time.sleep(86400)  # Sleep for a day at a time
    except KeyboardInterrupt:
        logger.info("Shutting down ML Service")
        server.stop(0)


if __name__ == "__main__":
    serve()
