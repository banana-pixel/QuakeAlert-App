#include "onset.h"

int64_t epochFromMonotonic(int64_t nowEpochMs, uint32_t nowMillis, uint32_t atMillis) {
    if (nowEpochMs <= 0) {
        return 0;
    }
    const uint32_t elapsed = nowMillis - atMillis;
    return nowEpochMs - static_cast<int64_t>(elapsed);
}
