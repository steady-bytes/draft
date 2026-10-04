# relay-pyannote-wrapper

Relay's own HTTP wrapper around [`pyannote.audio`](https://github.com/pyannote/pyannote-audio) for
Phase 8 (speaker diarization, archive only — see the implementation plan's Speech-to-text pipeline
section). There is no off-the-shelf `pyannote.audio` server the way `whisper.cpp` ships one; this
directory is that server, following the exact contract `service/diarize.go` expects. See
`server.py`'s own docstring for the full `POST /diarize` request/response shape.

## One-time setup

`pyannote.audio`'s pretrained diarization pipeline is gated on HuggingFace. This is a real,
one-time account setup step — nothing in this repo or in Claude Code can do it on your behalf:

1. Create a HuggingFace account (if you don't have one): https://huggingface.co/join
2. Accept the terms for both gated models this pipeline depends on:
   - https://huggingface.co/pyannote/speaker-diarization-3.1
   - https://huggingface.co/pyannote/segmentation-3.0
3. Create a **read** access token: https://huggingface.co/settings/tokens
4. Set it in the environment the wrapper runs in:
   ```shell
   export HF_TOKEN=hf_...
   ```

Without `HF_TOKEN` set, the wrapper starts fine and answers `/healthz`, but every `/diarize` call
returns `503` with a message explaining this — that's the expected behavior for an unconfigured
install, not a bug.

## Local dev

```shell
cd services/tooling/relay/pyannote-wrapper
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
export HF_TOKEN=hf_...   # see above
uvicorn server:app --host 127.0.0.1 --port 9310
```

Point Relay at it via `relay.pyannote.address` in `config.yaml` (defaults to
`http://127.0.0.1:9310`, matching the port above).

## Production

Same native-dev/container-prod split as `whisper.cpp` (see the plan's Speech-to-text pipeline
section): a Kubernetes sidecar container in Relay's own Pod spec, reachable over `localhost` inside
the shared Pod network namespace, `HF_TOKEN` supplied via a Secret rather than a plain env var.
Not yet wired into `scripts/run-local.sh`/`run-local-watch.sh` — do that once `HF_TOKEN` is
actually configured in this environment; until then, an absent sidecar just means every
`StopRecording` logs a diarization failure and leaves the transcript's `speaker_id`s empty, exactly
the same graceful degradation an unreachable `whisper.cpp` would get.
