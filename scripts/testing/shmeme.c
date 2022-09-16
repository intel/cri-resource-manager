// Copyright 2022 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This file implements interactive prompt for using shared memory.

#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>
#include <sys/shm.h>
#include <sys/types.h>

/*
#define SHM_HUGE_2MB    (21 << SHM_HUGE_SHIFT)
#define SHM_HUGE_1GB    (30 << SHM_HUGE_SHIFT)
*/

#define SHOWRUN(code) do { printf(#code "\n"); code; } while (0)

#define SHOWVAR(fmt, var) do { printf(#var ": " fmt "\n", var); } while (0)

#define CONST(fmt, const_type, const_name)                              \
    for (i=0; i<max_consts && (const_names[i] == 0 || strcmp(const_names[i], #const_name) != 0); ++i) { \
        if (const_names[i] == 0) {                                      \
            const_names[i]=malloc(strlen(#const_name));                 \
            strcpy(const_names[i], #const_name);                        \
            const_values_##const_type[i] = const_name;                  \
            break;                                                      \
        }                                                               \
    }                                                                   \
    if (strcmp(cmd, #const_name) == 0) {                                \
        SHOWVAR(fmt, const_name);                                       \
        cmd_handled = 1;                                                \
    }                                                                   \
    if (cmd_handled) { cmd_handled = 0; continue; }

#define VAR(fmt, var)                                       \
    for (i=0;i<max_vars;++i) {                              \
        if (vars[i] == 0) {                                 \
            vars[i]=malloc(strlen(#var "=" fmt));           \
            strcpy(vars[i], #var "=" fmt);                  \
            break;                                          \
        } else if (strcmp(vars[i], #var "=" fmt) == 0) {    \
            break;                                          \
        }                                                   \
    }                                                       \
    if ((strcmp(#fmt, "%s") == 0                            \
         && sscanf(cmd, #var "=" fmt, var) == 1)            \
        ||                                                  \
        (strcmp(#fmt, "%s") != 0                            \
         && sscanf(cmd, #var "=" fmt, &var) == 1)) {        \
        SHOWVAR(fmt, var);                                  \
        cmd_handled = 1;                                    \
    } else if (strcmp(cmd, #var) == 0) {                    \
        SHOWVAR(fmt, var);                                  \
        cmd_handled = 1;                                    \
    }                                                       \
    if (cmd_handled) { cmd_handled = 0; continue; }

#define VAR_CONST_EXPR(var, oper, const_type)                           \
    if (sscanf(cmd, #var #oper "%s", s) == 1 && const_index(s) >= 0) {  \
        var oper const_values_##const_type [const_index(s)];            \
        cmd_handled = 1;                                                \
    }                                                                   \
    if (cmd_handled) { cmd_handled = 0; continue; }

#define FUNC(func, code)                                \
    for (i=0;i<max_funcs;++i) {                         \
        if (func_names[i] == 0) {                       \
            func_names[i]=malloc(strlen(#func));        \
            strcpy(func_names[i], #func);               \
            break;                                      \
        } else if (strcmp(func_names[i], #func) == 0) { \
            break;                                      \
        }                                               \
    }                                                   \
    if (strcmp(cmd, #func) == 0) {                      \
        printf("%s() calls: " #code "\n", cmd);         \
        cmd_handled = 1;                                \
    } else if (strcmp(cmd, #func "()") == 0) {          \
        SHOWRUN(code);                                  \
        cmd_handled = 1;                                \
    }                                                   \
    if (cmd_handled) { cmd_handled = 0; continue; }

/* helpers */
#define max_vars 1024
#define max_consts 1024
#define max_funcs 1024

char *vars[max_vars];
char *const_names[max_consts];
char *func_names[max_funcs];
char s[1024];
int const_values_int[max_consts];

int const_index(char* const_name) {
    for (int i=0; i < max_consts && const_names[i] != 0; ++i) {
        if (strcmp(const_names[i], const_name) == 0) {
            return i;
        }
    }
    return -1;
}

int main(int argc, char** argv) {
    /* shmget in/out */
    key_t key = 0;
    int shmflg = 0;
    size_t size = 0;
    /* shmat in/out */
    int shmid = 0; /* returned by shmget */
    void* shmaddr = 0;
    void* addr = 0; /* returned by shmget */
    /* shmdt in/out */
    int err = 0;
    char file[1024];
    char c;
    char* p;
    int i = 0;
    char cmd[1024];
    int cmd_handled = 0;

    setvbuf(stdin, NULL, _IONBF, 0);
    setvbuf(stdout, NULL, _IONBF, 0);

    while (printf("> ") > 0 && scanf("%s", &cmd) == 1 && strcmp(cmd, "q") != 0) {
        CONST("0x%x", int, IPC_CREAT);
        CONST("0x%x", int, IPC_EXCL);
        CONST("0x%x", int, SHM_HUGETLB);
        CONST("0x%x", int, SHM_NORESERVE);
        CONST("0x%x", int, SHM_EXEC);
        CONST("0x%x", int, IPC_PRIVATE);

        VAR("0x%x", key);
        VAR("0x%x", shmflg);
        VAR("%d", size);
        VAR("0x%x", shmaddr);
        VAR("%d", shmid);
        VAR("0x%x", addr);
        VAR("0x%x", c);
        VAR("%d", err);
        VAR("%s", file);

        VAR_CONST_EXPR(shmflg, |=, int);

        FUNC(shmget, shmid = shmget(key, size, shmflg|0600));
        FUNC(shmat, addr = shmat(shmid, addr, shmflg));
        FUNC(shmdt, err = shmdt(addr));
        FUNC(shmctl-rm, err = shmctl(shmid, IPC_RMID, NULL));
        FUNC(write, for(p=addr;p<(char*)(addr+size);p+=4096){*p=c;});
        FUNC(strerror, printf("%s\n", strerror(errno)));

        if (strcmp(cmd, "help") == 0) {
            printf("variables and input format:\n");
            for (i=0; i<max_vars && vars[i] != 0; ++i) {
                printf("  %s\n", vars[i]);
            }
            printf("constants:\n");
            for (i=0; i<max_consts && const_names[i] != 0; ++i) {
                printf("  %s\n", const_names[i]);
            }
            printf("functions:\n");
            for (i=0; i<max_funcs && func_names[i] != 0; ++i) {
                printf("  %s\n", func_names[i]);
            }
            continue;
        }

        printf("error: ignoring bad command '%s'\n", cmd);
    }
}
