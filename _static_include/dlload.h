#ifndef __DLLOAD_H__
#define __DLLOAD_H__

#include <stdint.h>

void* OpenLibrary(const char *name);
void CloseLibrary(void *lib_handle);

void* SymbolFromName(void *lib_handle, const void *name);

#endif
