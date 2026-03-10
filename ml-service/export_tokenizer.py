"""
Export the Keras tokenizer.pickle to a framework-free tokenizer.json.

The JSON file contains only the word_index dict, which is all that's needed
for char-level text_to_sequences at inference time. This removes the runtime
dependency on TensorFlow/Keras for the gRPC serving code.

Usage:
    python export_tokenizer.py [input_pickle] [output_json]

Defaults:
    input_pickle = tokenizer.pickle
    output_json  = tokenizer.json
"""

import json
import os
import pickle
import sys


def export(
    pickle_path: str = "tokenizer.pickle", json_path: str = "tokenizer.json"
) -> None:
    if not os.path.exists(pickle_path):
        print(f"Error: {pickle_path} not found.")
        sys.exit(1)

    print(f"[*] Loading {pickle_path} ...")

    # The pickle contains a tensorflow.keras.preprocessing.text.Tokenizer.
    # We need Keras available to unpickle it. This script is meant to be run
    # once on a dev machine that HAS Keras installed, so the serving code
    # (guardian_grpc.py) never needs it again.
    try:
        with open(pickle_path, "rb") as f:
            tokenizer = pickle.load(f)
    except ModuleNotFoundError as e:
        print(f"Error: {e}")
        print(
            "This script must be run in an environment with tensorflow/keras installed"
        )
        print("so it can unpickle the Keras Tokenizer object.")
        sys.exit(1)

    word_index = tokenizer.word_index  # dict: char -> int

    # Preserve tokenizer config that matters for inference.
    data = {
        "word_index": word_index,
        "oov_token": getattr(tokenizer, "oov_token", "<UNK>"),
        "lower": getattr(tokenizer, "lower", True),
        "char_level": getattr(tokenizer, "char_level", True),
    }

    # The oov_token index (if present) is usually max(word_index.values()) or 1.
    # Store it explicitly so the lightweight tokenizer can use it.
    if data["oov_token"] and data["oov_token"] in word_index:
        data["oov_index"] = word_index[data["oov_token"]]
    else:
        data["oov_index"] = 0  # 0 = ignore unknown chars

    with open(json_path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

    size_kb = os.path.getsize(json_path) / 1024
    print(f"[+] Exported {len(word_index)} tokens to {json_path} ({size_kb:.1f} KB)")
    print(f"    oov_token={data['oov_token']!r}  oov_index={data['oov_index']}")
    print(f"    lower={data['lower']}  char_level={data['char_level']}")
    print("[+] Done. You can now use tokenizer.json without Keras.")


if __name__ == "__main__":
    inp = sys.argv[1] if len(sys.argv) > 1 else "tokenizer.pickle"
    out = sys.argv[2] if len(sys.argv) > 2 else "tokenizer.json"
    export(inp, out)
