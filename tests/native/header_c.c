#include <hrtcold/hrtcold.h>

#include <string.h>

int main(void) {
    hrtcold_limits limits;
    hrtcold_error error;

    hrtcold_limits_default(&limits);
    if (limits.max_container_bytes != (uint64_t)(16u * 1024u * 1024u)) {
        return 1;
    }
    if (limits.max_bundle_bytes != (uint64_t)(15u * 1024u * 1024u)) {
        return 2;
    }
    if (limits.max_json_depth != 64u || limits.max_json_string_bytes != (uint32_t)(1024u * 1024u)) {
        return 3;
    }

    hrtcold_error_clear(&error);
    if (error.code != HRTCOLD_OK || error.message[0] != '\0') {
        return 4;
    }
    if (strcmp(hrtcold_version_string(), HRTCOLD_VERSION_STRING) != 0) {
        return 5;
    }
    if (HRTCOLD_ABI_VERSION != 1 || HRTCOLD_FINGERPRINT_BUF_LEN != 65 ||
        HRTCOLD_SHA256_BUF_LEN != 65) {
        return 6;
    }
    return 0;
}
