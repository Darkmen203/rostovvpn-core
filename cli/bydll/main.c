#include <stdio.h>
#include <stdlib.h>

extern char* parseCli(int argc, char** argv);

int main(int argc, char** argv) {
    char* result = parseCli(argc, argv);
    if (result == NULL) {
        return 0;
    }

    puts(result);
    free(result);
    return 0;
}
