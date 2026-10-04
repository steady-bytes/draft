// Relay's real browser playback engine for StreamAudioOut's raw PCM16LE chunks
// (api/tooling/relay/v1/service.proto's AudioChunk doc). Hand-written JS, not web-sys typed-array
// juggling from Rust, for the same reason draft-ui's own dist/boot.js is hand-written: this is
// exactly the class of small, self-contained browser-API glue Dioxus apps in this repo already
// reach for JS directly for, rather than fighting wasm-bindgen's more verbose bindings for
// something a dozen lines of JS does plainly. Rust (views/library.rs) drives this via
// document::eval, passing each chunk as a base64 string (JSON has no binary type).
//
// Playback model: each incoming chunk becomes its own AudioBuffer, scheduled to start exactly
// when the previous one ends (a simple gapless queue) rather than waiting for the whole track --
// this is what makes it real *streaming* playback, matching StreamAudioOut's own real-time pacing
// server-side (rpc.go's streamTrack), not "buffer everything, then play."
window.relayAudio = (function () {
  let ctx = null;
  let gainNode = null;
  let nextStartTime = 0;
  let sampleRate = 16000;
  let channels = 1;
  let stopped = true;

  function ensureContext(rate, ch) {
    if (ctx && (sampleRate !== rate || channels !== ch)) {
      // A new track can have a different format than the last one -- tear down and rebuild rather
      // than trying to resample an existing context.
      ctx.close();
      ctx = null;
    }
    sampleRate = rate;
    channels = ch;
    if (!ctx) {
      ctx = new (window.AudioContext || window.webkitAudioContext)({ sampleRate: rate });
      gainNode = ctx.createGain();
      gainNode.connect(ctx.destination);
    }
    return ctx;
  }

  return {
    // Called once when playback starts (or resumes after a track change) -- resets the schedule
    // queue so a leftover tail from a previous track can't overlap the new one.
    start: function (rate, ch, volumePct) {
      ensureContext(rate, ch);
      gainNode.gain.value = Math.max(0, Math.min(1, volumePct / 100));
      nextStartTime = ctx.currentTime;
      stopped = false;
    },

    setVolume: function (volumePct) {
      if (gainNode) gainNode.gain.value = Math.max(0, Math.min(1, volumePct / 100));
    },

    // pcmBase64 is a chunk's raw PCM16LE bytes, base64-encoded. Decodes, converts each i16 sample
    // to the f32 in [-1, 1] Web Audio expects, and schedules it right after whatever's already
    // queued. Returns the chunk's own duration in seconds so Rust can advance its own position
    // display without a separate clock.
    pushChunk: function (pcmBase64) {
      if (stopped || !ctx) return 0;
      const binary = atob(pcmBase64);
      const bytes = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
      const view = new DataView(bytes.buffer);

      const frameCount = bytes.length / 2 / channels;
      if (frameCount <= 0) return 0;
      const buffer = ctx.createBuffer(channels, frameCount, sampleRate);
      for (let ch = 0; ch < channels; ch++) {
        const data = buffer.getChannelData(ch);
        for (let i = 0; i < frameCount; i++) {
          const sampleIndex = (i * channels + ch) * 2;
          data[i] = view.getInt16(sampleIndex, true) / 32768;
        }
      }

      const source = ctx.createBufferSource();
      source.buffer = buffer;
      source.connect(gainNode);
      const startAt = Math.max(nextStartTime, ctx.currentTime);
      source.start(startAt);
      const duration = frameCount / sampleRate;
      nextStartTime = startAt + duration;
      return duration;
    },

    // How far ahead of real time the schedule queue currently is -- used to throttle how fast
    // Rust requests more chunks from StreamAudioOut, so the queue doesn't grow unbounded if the
    // network delivers faster than real-time playback consumes it.
    bufferedAheadSeconds: function () {
      if (!ctx) return 0;
      return Math.max(0, nextStartTime - ctx.currentTime);
    },

    stop: function () {
      stopped = true;
      if (ctx) {
        ctx.close();
        ctx = null;
      }
    },
  };
})();
