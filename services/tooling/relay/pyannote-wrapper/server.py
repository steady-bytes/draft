"""Relay's own HTTP wrapper around pyannote.audio -- Relay's first actual Python component (see
docs/website/content/docs/architecture/relay-implementation-plan.md's Speech-to-text pipeline
section). There is no off-the-shelf pyannote.audio server the way whisper.cpp ships one, so this
file *is* the contract Relay's Go side (service/diarize.go) expects:

    POST /diarize
        multipart/form-data, one field "file": a WAV file (any sample rate/channels -- pyannote
        resamples internally; Relay's own recordings are 16kHz mono, but this doesn't assume that).
    -> 200 OK, JSON body: [{"start_ms": int, "end_ms": int, "speaker_label": str}, ...]
       (pyannote's own raw labels, e.g. "SPEAKER_00" -- Relay's own diarizeRecording, rpc.go, maps
       these to sequential "Speaker 1"/"Speaker 2" display labels; this wrapper doesn't rename them)
    -> 503, JSON body: {"error": "..."} if the pretrained pipeline never loaded (see below)

Batch only, by design -- see the plan's own "Diarization is batch-only, by design" decision:
pyannote's pretrained pipeline processes a complete audio file, there is no streaming variant.
This wrapper only ever needs to run once per finalized Recording (StopRecording, rpc.go), not
continuously.

Deployment: native process for local dev, a Kubernetes sidecar container in production -- the
exact same native-dev/container-prod split as whisper.cpp (see run-local-watch.sh's own whisper.cpp
block for the pattern to mirror here once this is wired into the launch scripts).

External prerequisite, gated model: `pyannote/speaker-diarization-3.1` (the pipeline loaded below)
is gated on HuggingFace -- loading it needs a HuggingFace account that has accepted the model's
terms, plus an access token set as HF_TOKEN in this process's environment. This is a one-time
setup step outside this repo (huggingface.co -> accept the model + pyannote/segmentation-3.0's own
terms -> create a read token -> `export HF_TOKEN=...`) -- nothing here can perform it on your
behalf. Without HF_TOKEN set, /diarize always returns 503 with a message saying so; this is the
expected, correct behavior for an unconfigured install, not a bug to silence.
"""

from __future__ import annotations

import os
import tempfile

from fastapi import FastAPI, File, HTTPException, UploadFile
from fastapi.responses import JSONResponse

app = FastAPI(title="relay-pyannote-wrapper")

# Loaded lazily on first request, not at import time: constructing the app (and answering to a
# health check) shouldn't require the gated model to already be reachable, and re-importing this
# module (e.g. under a reloader) shouldn't re-trigger a slow load every time.
_pipeline = None
_pipeline_load_error: str | None = None


def _load_pipeline():
    global _pipeline, _pipeline_load_error
    if _pipeline is not None or _pipeline_load_error is not None:
        return

    token = os.environ.get("HF_TOKEN")
    if not token:
        _pipeline_load_error = (
            "HF_TOKEN is not set. pyannote/speaker-diarization-3.1 is gated on HuggingFace -- "
            "create a HuggingFace account, accept the terms for pyannote/speaker-diarization-3.1 "
            "and pyannote/segmentation-3.0, create a read access token, and set it as HF_TOKEN in "
            "this process's environment before starting the wrapper. See the implementation "
            "plan's Speech-to-text pipeline section."
        )
        return

    try:
        from pyannote.audio import Pipeline

        _pipeline = Pipeline.from_pretrained(
            "pyannote/speaker-diarization-3.1", use_auth_token=token
        )
    except Exception as exc:  # noqa: BLE001 -- any load failure is reported the same way
        _pipeline_load_error = f"failed to load pyannote pipeline: {exc}"


@app.get("/healthz")
def healthz():
    """Reports whether the gated pipeline is actually usable, not just whether the process is up
    -- run-local-watch.sh's own wait_for_tcp only checks the port is open, so this is for a human
    checking why /diarize keeps 503ing, not part of that startup gate."""
    _load_pipeline()
    return {"pipeline_loaded": _pipeline is not None, "error": _pipeline_load_error}


@app.post("/diarize")
async def diarize(file: UploadFile = File(...)):
    _load_pipeline()
    if _pipeline is None:
        raise HTTPException(status_code=503, detail=_pipeline_load_error)

    data = await file.read()

    # pyannote's pipeline wants a file path (or a torchaudio-loadable stream); writing to a real
    # temp file is simpler and more robust across pyannote/torchaudio versions than trying to feed
    # it an in-memory buffer directly.
    with tempfile.NamedTemporaryFile(suffix=".wav") as tmp:
        tmp.write(data)
        tmp.flush()

        try:
            diarization = _pipeline(tmp.name)
        except Exception as exc:  # noqa: BLE001
            raise HTTPException(status_code=500, detail=f"diarization failed: {exc}") from exc

    segments = [
        {
            "start_ms": int(turn.start * 1000),
            "end_ms": int(turn.end * 1000),
            "speaker_label": speaker,
        }
        for turn, _, speaker in diarization.itertracks(yield_label=True)
    ]
    return JSONResponse(segments)
