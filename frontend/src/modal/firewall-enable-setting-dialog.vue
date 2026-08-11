<template>
  <teleport to="body">
    <transition enter-active-class="transition duration-200 ease-out" enter-from-class="opacity-0 scale-95"
      enter-to-class="opacity-100 scale-100" leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100 scale-100" leave-to-class="opacity-0 scale-95">
      <div v-if="showDialog" class="fixed inset-0 z-50 flex items-center justify-center p-4">
        <!-- Backdrop -->
        <div class="absolute inset-0 bg-black/50 backdrop-blur-sm" @click="close" />

        <!-- Card -->
        <div class="relative w-full max-w-sm overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-2xl">
          <!-- Header -->
          <div :class="[
            'flex items-center justify-between px-5 py-4',
            isDisableMode ? 'bg-amber-500' : 'bg-odoo',
          ]">
            <div class="flex items-center gap-2.5">
              <!-- Enable -->
              <svg v-if="!isDisableMode" class="h-5 w-5 flex-shrink-0 text-white" fill="none" viewBox="0 0 24 24"
                stroke="currentColor" stroke-width="2.5">
                <path stroke-linecap="round" stroke-linejoin="round"
                  d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
              </svg>

              <!-- Disable -->
              <svg v-else class="h-5 w-5 flex-shrink-0 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor"
                stroke-width="2.5">
                <path stroke-linecap="round" stroke-linejoin="round"
                  d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
              </svg>

              <span class="text-sm font-semibold tracking-wide text-white">
                {{ isDisableMode ? 'Disable Network Printing' : 'Enable Network Printing' }}
              </span>
            </div>

            <button v-if="!loading"
              class="cursor-pointer rounded-full p-1 text-white/70 transition-colors hover:bg-white/20 hover:text-white"
              @click="close">
              <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          <!-- Body -->
          <div class="max-h-[60vh] space-y-4 overflow-y-auto px-5 py-5">

            <!-- Authentication cancelled -->
            <div v-if="authCancelled" class="flex gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4">
              <svg class="mt-0.5 h-5 w-5 flex-shrink-0 text-amber-500" fill="none" viewBox="0 0 24 24"
                stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round"
                  d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>

              <div class="text-sm font-medium leading-relaxed text-amber-800">
                <p>Firewall configuration was cancelled.</p>
                <p class="mt-0.5 text-xs font-normal text-amber-700">
                  You can enable network printing later from the Settings menu.
                </p>
              </div>
            </div>

            <!-- Loading -->
            <div v-else-if="loading" class="flex flex-col items-center justify-center gap-3 py-8">
              <svg class="h-10 w-10 animate-spin text-odoo" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-20" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-80" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>

              <span class="text-sm font-medium text-gray-500">
                {{
                  isDisableMode
                    ? 'Disabling network printing...'
                    : 'Enabling network printing...'
                }}
              </span>
            </div>

            <!-- Disable -->
            <div v-else-if="isDisableMode" class="space-y-3">
              <div class="flex gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4">
                <svg class="mt-0.5 h-5 w-5 flex-shrink-0 text-amber-500" fill="none" viewBox="0 0 24 24"
                  stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round"
                    d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>

                <div>
                  <p class="text-sm font-semibold text-amber-800">
                    Network printing will be disabled
                  </p>
                  <p class="mt-1 text-xs leading-relaxed text-amber-700">
                    Other devices on your local network will no longer be able
                    to send print jobs to this device.
                  </p>
                </div>
              </div>

              <p class="px-1 text-xs leading-relaxed text-gray-500">
                You can enable network printing again at any time from Settings.
              </p>
            </div>

            <!-- Enable -->
            <div v-else class="space-y-3">

              <!-- Network printing -->
              <div class="flex items-start gap-3 rounded-xl border border-gray-100 bg-gray-50 p-4">
                <svg class="mt-0.5 h-5 w-5 flex-shrink-0 text-odoo" fill="none" viewBox="0 0 24 24"
                  stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round"
                    d="M17 17H17.01M7 17H7.01M5 9H19M5 9C4.46957 9 3.96086 9.21071 3.58579 9.58579 3.21071 9.96086 3 10.4696 3 11V17C3 17.5304 3.21071 18.0391 3.58579 18.4142 3.96086 18.7893 4.46957 19 5 19H19C19.5304 19 20.0391 18.7893 20.4142 18.4142 20.7893 18.0391 21 17.5304 21 17V11C21 10.4696 20.7893 9.96086 20.4142 9.58579 20.0391 9.21071 19 9 19 9M5 9L7 5H17L19 9" />
                </svg>

                <div>
                  <p class="text-sm font-semibold text-gray-700">
                    Network Printing
                  </p>
                  <p class="mt-0.5 text-xs leading-relaxed text-gray-500">
                    Allow other devices on your local network to send print
                    jobs to this device.
                  </p>
                </div>
              </div>

              <!-- Windows -->
              <div v-if="os === 'windows'"
                class="flex items-start gap-3 rounded-xl border border-blue-200 bg-blue-50 p-4">
                <svg class="h-5 w-5 flex-shrink-0 text-red-600" fill="none" viewBox="0 0 24 24" stroke="currentColor"
                  stroke-width="2.5">
                  <path stroke-linecap="round" stroke-linejoin="round"
                    d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                </svg>

                <div>
                  <p class="text-sm font-semibold text-red-800">
                    Windows Firewall
                  </p>

                  <p class="mt-1 text-xs leading-relaxed text-red-700">
                    Windows will configure an application-level firewall
                    permission to allow this application to receive print jobs
                    from other devices on your local network.
                  </p>
                </div>
              </div>

              <!-- Linux -->
              <div v-else-if="os === 'linux'"
                class="flex items-start gap-3 rounded-xl border border-orange-200 bg-orange-50 p-4">
                <svg class="h-5 w-5 flex-shrink-0 text-red-600" fill="none" viewBox="0 0 24 24" stroke="currentColor"
                  stroke-width="2.5">
                  <path stroke-linecap="round" stroke-linejoin="round"
                    d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                </svg>

                <div>
                  <p class="text-sm font-semibold text-orange-800">
                    Linux Firewall
                  </p>

                  <p class="mt-1 text-xs leading-relaxed text-orange-700">
                    A firewall rule will be created to allow incoming
                    connections on the network printing port. This is a
                    one-time setup.
                  </p>
                </div>
              </div>

              <!-- macOS -->
              <div v-else class="flex items-start gap-3 rounded-xl border border-red-200 bg-red-50 p-4">
                <svg class="h-5 w-5 flex-shrink-0 text-red-600" fill="none" viewBox="0 0 24 24" stroke="currentColor"
                  stroke-width="2.5">
                  <path stroke-linecap="round" stroke-linejoin="round"
                    d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                </svg>

                <div>
                  <p class="text-sm font-semibold text-red-800">
                    macOS Firewall
                  </p>

                  <p class="mt-1 text-xs leading-relaxed text-red-700">
                    This application cannot configure the macOS firewall
                    automatically.
                  </p>

                  <p class="mt-2 text-xs leading-relaxed text-red-700">
                    If the firewall is enabled, allow this application to
                    receive incoming connections in
                    <strong class="font-semibold text-red-800">
                      System Settings → Privacy &amp; Security → Firewall
                    </strong>.
                  </p>
                </div>
              </div>

              <!-- macOS additional information -->
              <div v-if="os === 'darwin'" class="flex gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4">
                <svg class="mt-0.5 h-5 w-5 flex-shrink-0 text-amber-500" fill="none" viewBox="0 0 24 24"
                  stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round"
                    d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77-1.333.192 3 1.732 3z" />
                </svg>

                <p class="text-xs leading-relaxed text-amber-800">
                  Clicking <strong>Enable</strong> enables network printing in
                  this application. You must allow the application through the
                  macOS firewall separately if required.
                </p>
              </div>

              <!-- Windows / Linux additional information -->
              <p v-else class="px-1 text-xs leading-relaxed text-gray-400">
                Choosing
                <strong class="text-gray-500">Not Now</strong>
                keeps the application running, but other devices will not be
                able to send print jobs until network access is enabled.
              </p>
            </div>

            <!-- Error -->
            <div v-if="error && !loading"
              class="flex items-start gap-2.5 rounded-xl border border-red-200 bg-red-50 px-4 py-3">
              <svg class="mt-0.5 h-4 w-4 flex-shrink-0 text-red-500" fill="none" viewBox="0 0 24 24"
                stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round"
                  d="M10 14l2-2m0 0l-2-2m2 2l2 2m-2-2l-2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <p class="text-xs font-medium text-red-700">{{ error }}</p>
            </div>
          </div>

          <!-- Footer -->
          <div class="flex gap-2.5 px-5 py-3">

            <!-- Authentication cancelled -->
            <template v-if="authCancelled">
              <button
                class="flex-1 cursor-pointer rounded-xl bg-gray-100 py-2.5 text-sm font-semibold text-gray-600 transition-colors hover:bg-gray-200"
                @click="close">
                Close
              </button>
            </template>

            <!-- Disable -->
            <template v-else-if="isDisableMode && !loading">
              <button
                class="flex-1 cursor-pointer rounded-xl bg-gray-100 py-2.5 text-sm font-semibold text-gray-600 transition-colors hover:bg-gray-200"
                @click="close">
                Cancel
              </button>

              <button
                class="flex-1 cursor-pointer rounded-xl bg-amber-500 py-2.5 text-sm font-semibold text-white shadow-md shadow-amber-500/25 transition-all hover:bg-amber-600"
                @click="handleDisable">
                Disable
              </button>
            </template>

            <!-- Enable -->
            <template v-else-if="!loading">
              <button
                class="flex-1 cursor-pointer rounded-xl bg-gray-100 py-2.5 text-sm font-semibold text-gray-600 transition-colors hover:bg-gray-200"
                @click="handleNotNow">
                Not Now
              </button>

              <button
                class="flex-1 cursor-pointer rounded-xl bg-odoo py-2.5 text-sm font-semibold text-white shadow-md shadow-odoo/25 transition-all hover:bg-odoo-dark"
                @click="handleEnable">
                Enable
              </button>
            </template>
          </div>
        </div>
      </div>
    </transition>
  </teleport>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { ConfigureFirewall, DisableFirewall } from '../../wailsjs/go/main/App'

const props = defineProps({
  os: String,
  /** 'enable' | 'disable' */
  mode: {
    type: String,
    default: 'enable',
  },
})

const emit = defineEmits(['notify', 'refresh', 'close'])

const isDisableMode = computed(() => props.mode === 'disable')

const showDialog = ref(false)
const loading = ref(false)
const error = ref(null)
const authCancelled = ref(false)

onMounted(() => {
  // Show immediately when rendered via v-if from parent (toggle flow)
  showDialog.value = true

  // Also respond to the Wails startup event (standalone usage)
  EventsOn('open-firewall-prompt', () => {
    showDialog.value = true
    authCancelled.value = false
    error.value = null
    loading.value = false
  })
})

// ── Enable flow ──────────────────────────────────────────────────────────────

async function handleEnable() {
  if (loading.value) return
  loading.value = true
  error.value = null
  authCancelled.value = false

  try {
    await ConfigureFirewall()
    emit('notify', 'Network printing enabled successfully.', 'success')
    emit('refresh')
    showDialog.value = false
  } catch (err) {
    const errorText = typeof err === 'string' ? err : err?.message || ''
    if (
      errorText.includes('authentication cancelled') ||
      errorText.includes('canceled by the user') ||
      errorText.includes('1223')
    ) {
      authCancelled.value = true
    } else {
      error.value = errorText || 'Failed to configure firewall rule.'
    }
  } finally {
    loading.value = false
  }
}

function handleNotNow() {
  if (loading.value) return
  showDialog.value = false
  emit('close')
}

// ── Disable flow ─────────────────────────────────────────────────────────────

async function handleDisable() {
  if (loading.value) return
  loading.value = true
  error.value = null

  try {
    await DisableFirewall()
    emit('notify', 'Other devices can no longer send print jobs.', 'success')
    emit('refresh')
    showDialog.value = false
  } catch (err) {
    const errorText = typeof err === 'string' ? err : err?.message || ''
    error.value = errorText || 'Failed to disable the firewall rule.'
  } finally {
    loading.value = false
  }
}

// ── Shared ───────────────────────────────────────────────────────────────────

function close() {
  if (loading.value) return
  showDialog.value = false
  emit('close')
}
</script>
