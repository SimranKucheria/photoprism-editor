<template>
  <div class="p-tab-photo-ai">
    <div class="pa-4">
      <v-textarea
        v-model="prompt"
        :label="$gettext('Describe your edit')"
        :placeholder="$gettext('Example: Crop to center, increase exposure, and enhance details')"
        :disabled="busy"
        rows="4"
        auto-grow
        class="input-prompt"
      ></v-textarea>

      <div class="d-flex ga-2 flex-wrap mt-2">
        <v-btn color="highlight" variant="flat" class="action-run-agent" :disabled="busy || !prompt" @click.stop="runAgent">
          {{ $gettext("Run Agent") }}
        </v-btn>
        <v-btn color="button" variant="flat" class="action-reset" :disabled="busy || !planId" @click.stop="resetPlan">
          {{ $gettext("Reset") }}
        </v-btn>
      </div>

      <div v-if="busy" class="mt-4">
        <v-progress-linear indeterminate color="highlight"></v-progress-linear>
      </div>

      <v-alert v-if="status" class="mt-4" density="comfortable" type="info" variant="tonal"> {{ $gettext("Status") }}: {{ statusLabel(status) }} </v-alert>

      <div v-if="operations.length" class="mt-4">
        <h4 class="text-subtitle-2">{{ $gettext("Planned Operations") }}</h4>
        <v-list density="comfortable" class="plan-list">
          <v-list-item v-for="(operation, index) in operations" :key="`${operation.tool}-${index}`">
            <v-list-item-title>{{ toolLabel(operation.tool) }}</v-list-item-title>
          </v-list-item>
        </v-list>
      </div>

      <div v-if="results.length" class="mt-4">
        <h4 class="text-subtitle-2">{{ $gettext("Execution Results") }}</h4>
        <v-list density="comfortable" class="plan-list">
          <v-list-item v-for="(result, index) in results" :key="`${result.tool}-${result.status}-${index}`">
            <v-list-item-title>{{ toolLabel(result.tool) }} - {{ statusLabel(result.status) }}</v-list-item-title>
            <v-list-item-subtitle v-if="result.message">{{ result.message }}</v-list-item-subtitle>
          </v-list-item>
        </v-list>
      </div>

      <div class="mt-4">
        <h4 class="text-subtitle-2">{{ $gettext("Preview") }}</h4>
        <img :src="previewUrl" :style="previewStyle" :alt="$gettext('Photo preview')" style="border-radius: 8px; max-width: 100%; width: 100%" />
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: "PTabPhotoAI",
  props: {
    uid: {
      type: String,
      default: "",
    },
  },
  data() {
    return {
      view: this.$view.getData(),
      prompt: "",
      planId: "",
      approved: false,
      operations: [],
      results: [],
      status: "",
      busy: false,
      previewVersion: Date.now(),
      previewReloadTimer: null,
    };
  },
  computed: {
    previewUrl() {
      if (!this.view?.model) {
        return "";
      }

      const baseUrl = this.view.model.thumbnailUrl("fit_720");
      const separator = baseUrl.includes("?") ? "&" : "?";

      return `${baseUrl}${separator}v=${this.previewVersion}`;
    },
    previewStyle() {
      const filters = [];
      let rotateDeg = 0;

      // Only reflect operations that actually succeeded on the server.
      this.results
        .filter((r) => r.status === "succeeded")
        .forEach((r) => {
          const matchedOp = this.operations.find((op) => op.tool === r.tool) || {};
          const tool = r.tool.toLowerCase();
          const params = matchedOp.params || {};

          if (tool === "rotate") {
            const degrees = Number(params.degrees || 0);
            if (!Number.isNaN(degrees)) {
              rotateDeg += degrees;
            }
          } else if (tool === "exposure") {
            const value = Number(params.value || 0);
            if (!Number.isNaN(value)) {
              filters.push(`brightness(${Math.max(0.2, 1 + value)})`);
            }
          } else if (tool === "contrast") {
            const value = Number(params.value || 0);
            if (!Number.isNaN(value)) {
              filters.push(`contrast(${Math.max(0.2, 1 + value)})`);
            }
          } else if (tool === "saturation") {
            const value = Number(params.value || 0);
            if (!Number.isNaN(value)) {
              filters.push(`saturate(${Math.max(0.2, 1 + value)})`);
            }
          } else if (tool === "temperature") {
            const value = Number(params.value || 0);
            if (!Number.isNaN(value) && value !== 0) {
              const hue = value > 0 ? -6 : 6;
              filters.push(`hue-rotate(${hue}deg)`);
            }
          }
        });

      return {
        filter: filters.length ? filters.join(" ") : "none",
        transform: rotateDeg ? `rotate(${rotateDeg}deg)` : "none",
      };
    },
  },
  watch: {
    uid() {
      this.clearState();
    },
  },
  beforeUnmount() {
    this.clearPreviewReloadTimer();
  },
  methods: {
    statusLabel(status) {
      const map = {
        applied: this.$gettext("Applied"),
        "partially-applied": this.$gettext("Partially Applied"),
        failed: this.$gettext("Failed"),
        running: this.$gettext("Running"),
        succeeded: this.$gettext("Succeeded"),
        skipped: this.$gettext("Skipped"),
        reset: this.$gettext("Reset"),
      };
      return map[status] || status;
    },
    toolLabel(tool) {
      const map = {
        rotate: this.$gettext("Rotate"),
        crop: this.$gettext("Crop"),
        auto_crop_straighten: this.$gettext("Auto Crop & Straighten"),
        exposure: this.$gettext("Exposure"),
        contrast: this.$gettext("Contrast"),
        saturation: this.$gettext("Saturation"),
        temperature: this.$gettext("Temperature"),
        inpaint_object: this.$gettext("Inpaint Object"),
        outpaint_canvas: this.$gettext("Outpaint Canvas"),
        transfer_style: this.$gettext("Transfer Style"),
        replace_background: this.$gettext("Replace Background"),
        version_checkpoint: this.$gettext("Version Checkpoint"),
        metadata_sync: this.$gettext("Metadata Sync"),
      };
      return map[tool] || tool;
    },
    clearPreviewReloadTimer() {
      if (this.previewReloadTimer) {
        clearTimeout(this.previewReloadTimer);
        this.previewReloadTimer = null;
      }
    },
    clearState() {
      this.clearPreviewReloadTimer();
      this.prompt = "";
      this.planId = "";
      this.approved = false;
      this.operations = [];
      this.results = [];
      this.status = "";
      this.busy = false;
      this.previewVersion = Date.now();
    },
    setPlan(data) {
      this.planId = data?.planId || "";
      this.approved = !!data?.approved;
      this.operations = Array.isArray(data?.operations) ? data.operations : [];
      this.results = Array.isArray(data?.results) ? data.results : [];
      this.status = data?.status || "";
      this.previewVersion = Date.now();
    },
    reloadEditedPreview() {
      if (!this.view?.model?.load) {
        this.previewVersion = Date.now();
        return Promise.resolve();
      }

      this.clearPreviewReloadTimer();

      return this.view.model
        .load()
        .then(() => {
          this.view.model.refreshFileAttr();
          this.previewVersion = Date.now();

          // Refresh one more time after a short delay in case thumbnail generation lags.
          this.previewReloadTimer = setTimeout(() => {
            this.previewVersion = Date.now();
            this.previewReloadTimer = null;
          }, 1500);
        })
        .catch(() => {
          this.previewVersion = Date.now();
        });
    },
    runAgent() {
      if (!this.view?.model || !this.prompt) {
        return;
      }

      this.busy = true;

      return this.view.model
        .generateEditPlan(this.prompt)
        .then((data) => {
          this.setPlan(data);

          if (data?.status === "applied" || data?.status === "partially-applied") {
            return this.reloadEditedPreview();
          }

          return Promise.resolve();
        })
        .then(() => {
          this.$notify.success(this.$gettext("Agent run completed"));
        })
        .catch((err) => this.$notify.error(err))
        .finally(() => {
          this.busy = false;
        });
    },
    resetPlan() {
      if (!this.view?.model || !this.planId) {
        return;
      }

      this.busy = true;

      return this.view.model
        .resetEditPlan(this.planId)
        .then(() => {
          this.clearState();
          this.$notify.success(this.$gettext("Edit plan reset"));
        })
        .catch((err) => this.$notify.error(err))
        .finally(() => {
          this.busy = false;
        });
    },
  },
};
</script>
