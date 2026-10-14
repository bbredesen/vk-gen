
#include <stdio.h>
#include <stdint.h>
#include "dlload.h"

#ifndef _WIN32
#include <dlfcn.h>


void* OpenLibrary(const char *name) {
	void* lib_handle = dlopen(name, RTLD_LOCAL|RTLD_LAZY);
	if (!lib_handle) {
		printf("Unable to load %s: %s\n", name, dlerror());
		return NULL;
	}
	return lib_handle;
}

void CloseLibrary(void *lib_handle) {
	if (dlclose(lib_handle) != 0) {
		printf("Problem closing library: %s", dlerror());
	}
}

void* SymbolFromName(void *lib_handle, const void *name) {
	return dlsym(lib_handle, (const char*) name);
}


#endif


#ifdef _WIN32

#include <windows.h>

void* OpenLibrary(const char *name) {
	return LoadLibrary(name);
}

void* SymbolFromName(void *lib_handle, const void *name) {
	return GetProcAddress(lib_handle, name);
}

void CloseLibrary(void *lib_handle) {
	FreeLibrary(lib_handle);
}


#endif
