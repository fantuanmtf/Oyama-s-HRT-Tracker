#include <hrtcold/hrtcold.h>

#include <cstring>
#include <type_traits>

static_assert(std::is_standard_layout<hrtcold_error>::value,
              "hrtcold_error must remain standard-layout for C interop");
static_assert(std::is_standard_layout<hrtcold_limits>::value,
              "hrtcold_limits must remain standard-layout for C interop");
static_assert(sizeof(hrtcold_error{}.message) == 256,
              "error message size is part of the v1 ABI");

int main() {
    hrtcold_limits limits{};
    hrtcold_error error{};

    hrtcold_limits_default(&limits);
    if (limits.max_container_bytes != (16u * 1024u * 1024u)) {
        return 1;
    }
    if (limits.max_bundle_bytes != (15u * 1024u * 1024u)) {
        return 2;
    }

    hrtcold_error_clear(&error);
    if (error.code != HRTCOLD_OK || error.message[0] != '\0') {
        return 3;
    }
    if (std::strcmp(hrtcold_version_string(), HRTCOLD_VERSION_STRING) != 0) {
        return 4;
    }
    return 0;
}
