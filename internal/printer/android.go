//go:build android

package printer

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

static JavaVM* g_printer_vm = NULL;
static jclass g_wails_bridge_cls = NULL;
static jmethodID g_mid_list_usb = NULL;
static jmethodID g_mid_print_usb = NULL;

jint JNI_OnLoad(JavaVM* vm, void* reserved) {
    g_printer_vm = vm;
    JNIEnv* env = NULL;
    if ((*vm)->GetEnv(vm, (void**)&env, JNI_VERSION_1_6) == JNI_OK && env != NULL) {
        jclass local_cls = (*env)->FindClass(env, "com/wails/app/WailsBridge");
        if (local_cls) {
            g_wails_bridge_cls = (jclass)(*env)->NewGlobalRef(env, local_cls);
            (*env)->DeleteLocalRef(env, local_cls);
            if (g_wails_bridge_cls) {
                g_mid_list_usb = (*env)->GetStaticMethodID(env, g_wails_bridge_cls, "getUSBPrintersJson", "()Ljava/lang/String;");
                g_mid_print_usb = (*env)->GetStaticMethodID(env, g_wails_bridge_cls, "writeUSBPrinter", "(Ljava/lang/String;)Ljava/lang/String;");
            }
        } else {
            (*env)->ExceptionClear(env);
        }
    }
    return JNI_VERSION_1_6;
}

static JNIEnv* get_jni_env(int* attached) {
    *attached = 0;
    if (!g_printer_vm) return NULL;
    JNIEnv* env = NULL;
    jint res = (*g_printer_vm)->GetEnv(g_printer_vm, (void**)&env, JNI_VERSION_1_6);
    if (res == JNI_EDETACHED) {
        if ((*g_printer_vm)->AttachCurrentThread(g_printer_vm, &env, NULL) == JNI_OK) {
            *attached = 1;
        } else {
            return NULL;
        }
    }
    return env;
}

static void release_jni_env(int attached) {
    if (attached && g_printer_vm) {
        (*g_printer_vm)->DetachCurrentThread(g_printer_vm);
    }
}

static char* call_java_list_usb() {
    int attached = 0;
    JNIEnv* env = get_jni_env(&attached);
    if (!env) return NULL;

    if (!g_wails_bridge_cls || !g_mid_list_usb) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        release_jni_env(attached);
        return NULL;
    }

    jstring jres = (jstring)(*env)->CallStaticObjectMethod(env, g_wails_bridge_cls, g_mid_list_usb);
    if ((*env)->ExceptionCheck(env)) {
        (*env)->ExceptionClear(env);
        release_jni_env(attached);
        return NULL;
    }

    if (!jres) {
        release_jni_env(attached);
        return NULL;
    }

    const char* str = (*env)->GetStringUTFChars(env, jres, NULL);
    char* result = NULL;
    if (str) {
        result = strdup(str);
        (*env)->ReleaseStringUTFChars(env, jres, str);
    }
    (*env)->DeleteLocalRef(env, jres);
    release_jni_env(attached);
    return result;
}

static char* call_java_print_usb(const char* json_arg) {
    int attached = 0;
    JNIEnv* env = get_jni_env(&attached);
    if (!env) return NULL;

    if (!g_wails_bridge_cls || !g_mid_print_usb) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        release_jni_env(attached);
        return NULL;
    }

    jstring jarg = (*env)->NewStringUTF(env, json_arg);
    jstring jres = (jstring)(*env)->CallStaticObjectMethod(env, g_wails_bridge_cls, g_mid_print_usb, jarg);
    if ((*env)->ExceptionCheck(env)) {
        (*env)->ExceptionClear(env);
        if (jarg) (*env)->DeleteLocalRef(env, jarg);
        release_jni_env(attached);
        return NULL;
    }

    if (jarg) (*env)->DeleteLocalRef(env, jarg);
    if (!jres) {
        release_jni_env(attached);
        return NULL;
    }

    const char* str = (*env)->GetStringUTFChars(env, jres, NULL);
    char* result = NULL;
    if (str) {
        result = strdup(str);
        (*env)->ReleaseStringUTFChars(env, jres, str);
    }
    (*env)->DeleteLocalRef(env, jres);
    release_jni_env(attached);
    return result;
}
*/
import "C"

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
	"unsafe"

	"epos-proxy/internal/logger"
)

type ConnKind int

const (
	ConnKindUSB ConnKind = iota
	ConnKindLAN
)

const (
	QueueSize    = 100
	WriteTimeout = 5 * time.Second
	ChunkSize    = 8 * 1024 // 8 KB
)

var (
	ErrNotFound  = errors.New("printer not found")
	ErrQueueFull = errors.New("printer queue is full")
)

type JobResult struct {
	OK  bool
	Err error
}

type JobFunc func(p *Printer) JobResult

type Job struct {
	run   JobFunc
	reply chan JobResult
}

type Info struct {
	Id   string
	Name string
	Type Type
}

type UnavailableInfo struct {
	Name  string
	Error string
}

type Printers struct {
	Available   []Info
	Unavailable []UnavailableInfo
}

type androidUSBDevice struct {
	Path   string `json:"path"`
	VidPid string `json:"vidPid"`
	Serial string `json:"serial"`
	Name   string `json:"name"`
}

type androidPrintResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func ListUSBPrinters() (*Printers, error) {
	cJson := C.call_java_list_usb()
	if cJson == nil {
		return &Printers{
			Available:   make([]Info, 0),
			Unavailable: make([]UnavailableInfo, 0),
		}, nil
	}
	rawJson := C.GoString(cJson)
	C.free(unsafe.Pointer(cJson))

	var devices []androidUSBDevice
	if err := json.Unmarshal([]byte(rawJson), &devices); err != nil {
		logger.Errorf("Failed to parse Android USB printers json: %v", err)
		return &Printers{
			Available:   make([]Info, 0),
			Unavailable: make([]UnavailableInfo, 0),
		}, nil
	}

	available := make([]Info, 0, len(devices))
	for _, dev := range devices {
		pType := getPrinterType(dev.VidPid)
		printerID, err := encodePrinterID(&LibUsbPrinter{
			VidPid: dev.VidPid,
			Serial: dev.Serial,
			Path:   dev.Path,
		})
		if err != nil {
			logger.Warnf("Failed to encode Android USB printer ID: %v", err)
			continue
		}
		available = append(available, Info{
			Id:   printerID,
			Name: dev.Name,
			Type: pType,
		})
	}

	return &Printers{
		Available:   available,
		Unavailable: make([]UnavailableInfo, 0),
	}, nil
}

type Printer struct {
	connectionType ConnKind
	lanIP          string
	usbID          *ID
	mu             sync.Mutex
	tcpConn        net.Conn
	jobs           chan Job
}

func newPrinter(id string) *Printer {
	if lanIP, ok := DecodeLANPrinterID(id); ok {
		p := &Printer{
			connectionType: ConnKindLAN,
			lanIP:          lanIP,
			jobs:           make(chan Job, QueueSize),
		}
		go p.loop()
		return p
	}

	decodedID, err := decodePrinterID(id)
	if err != nil {
		logger.Errorf("Failed to decode USB printer ID: %s, error: %v", id, err)
	}

	p := &Printer{
		connectionType: ConnKindUSB,
		usbID:          decodedID,
		jobs:           make(chan Job, QueueSize),
	}
	go p.loop()
	return p
}

func (p *Printer) ensureOpen() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.connectionType == ConnKindLAN {
		if p.tcpConn != nil {
			return nil
		}
		dialer := net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.Dial("tcp", net.JoinHostPort(p.lanIP, "9100"))
		if err != nil {
			return fmt.Errorf("failed to connect to LAN printer at %s:9100: %w", p.lanIP, err)
		}
		p.tcpConn = conn
		return nil
	}

	if p.connectionType == ConnKindUSB {
		if p.usbID == nil {
			return errors.New("USB printer ID is invalid")
		}
		return nil
	}

	return ErrNotFound
}

func (p *Printer) Write(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.connectionType == ConnKindLAN {
		if p.tcpConn == nil {
			return errors.New("LAN connection not open")
		}
		if err := p.tcpConn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
			return fmt.Errorf("failed to set write deadline: %w", err)
		}
		totalWritten := 0
		for totalWritten < len(data) {
			end := totalWritten + ChunkSize
			if end > len(data) {
				end = len(data)
			}
			n, err := p.tcpConn.Write(data[totalWritten:end])
			totalWritten += n
			if err != nil {
				_ = p.tcpConn.Close()
				p.tcpConn = nil
				return fmt.Errorf("LAN write failed: %w", err)
			}
		}
		return nil
	}

	if p.connectionType == ConnKindUSB {
		if p.usbID == nil {
			return errors.New("USB printer ID not available")
		}
		req := map[string]string{
			"path":   p.usbID.Path,
			"vidPid": p.usbID.VidPid,
			"serial": p.usbID.Serial,
			"data":   base64.StdEncoding.EncodeToString(data),
		}
		reqBytes, _ := json.Marshal(req)
		cReq := C.CString(string(reqBytes))
		cRes := C.call_java_print_usb(cReq)
		C.free(unsafe.Pointer(cReq))

		if cRes == nil {
			return errors.New("failed to call Android printUSB via JNI")
		}
		resStr := C.GoString(cRes)
		C.free(unsafe.Pointer(cRes))

		var res androidPrintResult
		if err := json.Unmarshal([]byte(resStr), &res); err != nil {
			return fmt.Errorf("failed to parse printUSB response: %w", err)
		}
		if !res.OK {
			return errors.New(res.Error)
		}
		return nil
	}

	return ErrNotFound
}

func (p *Printer) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.tcpConn != nil {
		_ = p.tcpConn.Close()
		p.tcpConn = nil
	}
}

func (p *Printer) Enqueue(run JobFunc, reply chan JobResult) error {
	select {
	case p.jobs <- Job{run: run, reply: reply}:
		return nil
	default:
		return ErrQueueFull
	}
}

func (p *Printer) loop() {
	for job := range p.jobs {
		result := job.run(p)
		if job.reply != nil {
			job.reply <- result
		}
	}
}
