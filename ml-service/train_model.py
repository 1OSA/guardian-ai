import os
import pickle

import numpy as np
import pandas as pd
import tensorflow as tf
from sklearn.model_selection import train_test_split
from sklearn.utils import class_weight
from tensorflow.keras.layers import (
    LSTM,
    Bidirectional,
    Conv1D,
    Dense,
    Dropout,
    Embedding,
    Flatten,
    MaxPooling1D,
)
from tensorflow.keras.models import Sequential
from tensorflow.keras.preprocessing.sequence import pad_sequences
from tensorflow.keras.preprocessing.text import Tokenizer
from tensorflow.keras.utils import to_categorical

# --- CONFIG ---
MAX_LEN = 75
EMBED_DIM = 64  # Increased from 32 to capture more nuance
EPOCHS = 12  # Increased to force convergence
BATCH_SIZE = 64  # Smaller batch size = more updates per epoch


def train():
    if not os.path.exists("guardian_dataset_full.csv"):
        print("Error: Dataset not found.")
        return

    print("Loading dataset...")
    df = pd.read_csv("guardian_dataset_full.csv")
    df["domain"] = df["domain"].astype(str)

    # --- STEP 1: FORCE-FEED KNOWLEDGE (Data Injection) ---
    print("Injecting High-Value Targets (Brand Immunization)...")

    # 1. Protect the Real Brands (Make sure it knows Google is Safe)
    safe_brands = [
        "google.com",
        "microsoft.com",
        "amazon.com",
        "apple.com",
        "facebook.com",
        "netflix.com",
        "mu.edu.sa",
    ]
    safe_injection = []
    for brand in safe_brands:
        safe_injection.extend([(brand, 0)] * 200)  # Show 'google.com' 200 times as Safe

    # 2. Punish the Lookalikes (rnicrosoft, g0ogle)
    # We explicitly generate the 'rn' and '0' cases here
    bad_injection = []
    bad_patterns = [
        ("rnicrosoft.com", 2),
        ("rnicrosoft.com", 2),
        ("micr0soft.com", 2),
        ("g0ogle.com", 2),
        ("goog1e.com", 2),
        ("qooqle.com", 2),
        ("faceb00k.com", 2),
        ("facebook-login.com", 2),
        ("arnazon.com", 2),
        ("amaz0n.com", 2),
        ("app1e.com", 2),
        ("apple-support.com", 2),
        ("secure-account-verification-paypal.com", 2),
        ("secure-account-verification-paypal.com", 2),
        ("apple-id-support-urgent.net", 2),
        ("apple-id-support-urgent.net", 2),
        ("microsoft-online-security.com", 2),
        ("microsoft-online-security.com", 2),
    ]
    for pattern in bad_patterns:
        bad_injection.extend([pattern] * 200)

    # Add to dataframe
    df_inject = pd.DataFrame(
        safe_injection + bad_injection, columns=["domain", "label"]
    )
    df = pd.concat([df, df_inject], ignore_index=True)
    df = df.sample(frac=1).reset_index(drop=True)

    print(f"Training on {len(df)} domains...")

    X_raw = df["domain"].values
    y_raw = df["label"].values

    # --- STEP 2: TOKENIZER ---
    # filters='' is CRITICAL for Cyrillic/Special chars
    tokenizer = Tokenizer(char_level=True, lower=True, filters="", oov_token="<UNK>")
    tokenizer.fit_on_texts(X_raw)

    with open("tokenizer.pickle", "wb") as f:
        pickle.dump(tokenizer, f)

    vocab_size = len(tokenizer.word_index) + 1
    sequences = tokenizer.texts_to_sequences(X_raw)
    X = pad_sequences(sequences, maxlen=MAX_LEN, padding="post")
    y = to_categorical(y_raw, num_classes=3)

    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.2, random_state=42
    )

    # --- STEP 3: CALCULATE CLASS WEIGHTS ---
    # This fixes the "Coin Flip" issue. We tell the model that Class 2 (Phishing) is rare and important.
    y_integers = np.argmax(y_train, axis=1)
    class_weights = class_weight.compute_class_weight(
        class_weight="balanced", classes=np.unique(y_integers), y=y_integers
    )
    # MANUALLY boost Class 2 (Phishing) importance by 5x
    weights_dict = {0: 1.0, 1: 1.0, 2: 5.0}
    print(f"Class Weights applied: {weights_dict}")

    # --- STEP 4: DEEPER ARCHITECTURE ---
    print("Building Deep CNN-LSTM...")
    model = Sequential()
    model.add(Embedding(vocab_size, EMBED_DIM, input_length=MAX_LEN))

    # Conv Layer 1: Look for 2-char patterns (like 'rn')
    model.add(Conv1D(filters=128, kernel_size=2, activation="relu", padding="same"))
    # Conv Layer 2: Look for 3-char patterns (like '00g')
    model.add(Conv1D(filters=128, kernel_size=3, activation="relu", padding="same"))
    model.add(MaxPooling1D(pool_size=2))

    model.add(Bidirectional(LSTM(128, return_sequences=False)))
    model.add(Dropout(0.5))

    model.add(Dense(64, activation="relu"))  # Extra thinking layer
    model.add(Dense(3, activation="softmax"))

    model.compile(
        loss="categorical_crossentropy", optimizer="adam", metrics=["accuracy"]
    )

    # --- STEP 5: TRAIN ---
    model.fit(
        X_train,
        y_train,
        epochs=EPOCHS,
        batch_size=BATCH_SIZE,
        validation_data=(X_test, y_test),
        class_weight=weights_dict,  # <--- Apply the penalty logic
    )

    model.save("guardian_model.h5")

    # --- LIVE TEST ---
    print("\n--- FINAL VERIFICATION ---")
    test_domains = [
        "google.com",
        "gоogle.com",
        "rnicrosoft.com",
        "kzxvqy.net",
        "mu.edu.sa",
    ]
    seqs = tokenizer.texts_to_sequences(test_domains)
    padded = pad_sequences(seqs, maxlen=MAX_LEN, padding="post")
    preds = model.predict(padded)

    labels = ["Safe", "DGA", "Phishing"]

    print(f"{'DOMAIN':<20} | {'VERDICT':<10} | {'CONFIDENCE'}")
    print("-" * 45)
    for i, domain in enumerate(test_domains):
        verdict = np.argmax(preds[i])
        conf = preds[i][verdict] * 100
        print(f"{domain:<20} | {labels[verdict]:<10} | {conf:.1f}%")


if __name__ == "__main__":
    train()
