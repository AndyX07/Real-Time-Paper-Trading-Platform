#pragma once

#include <cstdint>
#include <stdexcept>
#include <string>
#include <string_view>

constexpr int64_t PRICE_SCALE = 1e10; // 10 decimal places
constexpr int PRICE_SCALE_DIGITS = 10;

constexpr int64_t QUANTITY_SCALE = 1e10; // 10 decimal places
constexpr int QUANTITY_SCALE_DIGITS = 10;

inline bool priceScaleSupports(int pairDecimals) { return pairDecimals <= PRICE_SCALE_DIGITS; }
inline bool quantityScaleSupports(int lotDecimals) { return lotDecimals <= QUANTITY_SCALE_DIGITS; }

namespace price_detail {

inline int64_t parseFixedPoint(std::string_view s, int64_t scale, int scaleDigits) {
    if (s.empty()) throw std::invalid_argument("parseFixedPoint: empty string");

    bool negative = false;
    size_t pos = 0;
    if (s[0] == '-' || s[0] == '+') {
        negative = (s[0] == '-');
        pos = 1;
    }

    size_t dot = s.find('.', pos);
    std::string_view intPart = (dot == std::string_view::npos) ? s.substr(pos) : s.substr(pos, dot - pos);
    std::string_view fracPart = (dot == std::string_view::npos) ? std::string_view{} : s.substr(dot + 1);

    if (fracPart.size() > static_cast<size_t>(scaleDigits)) {
        throw std::invalid_argument("parseFixedPoint: more fractional digits than scale supports");
    }

    auto digitsToInt = [](std::string_view digits) {
        int64_t value = 0;
        for (char c : digits) {
            if (c < '0' || c > '9') throw std::invalid_argument("parseFixedPoint: invalid character");
            value = value * 10 + (c - '0');
        }
        return value;
    };

    int64_t intValue = digitsToInt(intPart);
    int64_t fracValue = digitsToInt(fracPart);
    for (size_t i = fracPart.size(); i < static_cast<size_t>(scaleDigits); ++i) {
        fracValue *= 10;
    }

    int64_t ticks = intValue * scale + fracValue;
    return negative ? -ticks : ticks;
}

inline std::string formatFixedPoint(int64_t ticks, int64_t scale, int scaleDigits) {
    bool negative = ticks < 0;
    uint64_t absTicks = negative ? static_cast<uint64_t>(-ticks) : static_cast<uint64_t>(ticks);
    uint64_t intPart = absTicks / static_cast<uint64_t>(scale);
    uint64_t fracPart = absTicks % static_cast<uint64_t>(scale);

    std::string fracStr = std::to_string(fracPart);
    if (fracStr.size() < static_cast<size_t>(scaleDigits)) {
        fracStr.insert(0, static_cast<size_t>(scaleDigits) - fracStr.size(), '0');
    }

    return (negative ? "-" : "") + std::to_string(intPart) + "." + fracStr;
}

}

struct Price {
    int64_t ticks = 0;

    auto operator<=>(const Price&) const = default;

    static Price fromDecimalString(std::string_view s) {
        return Price{price_detail::parseFixedPoint(s, PRICE_SCALE, PRICE_SCALE_DIGITS)};
    }
    std::string toDecimalString() const {
        return price_detail::formatFixedPoint(ticks, PRICE_SCALE, PRICE_SCALE_DIGITS);
    }
};

struct Quantity {
    int64_t ticks = 0;

    auto operator<=>(const Quantity&) const = default;

    static Quantity fromDecimalString(std::string_view s) {
        return Quantity{price_detail::parseFixedPoint(s, QUANTITY_SCALE, QUANTITY_SCALE_DIGITS)};
    }
    std::string toDecimalString() const {
        return price_detail::formatFixedPoint(ticks, QUANTITY_SCALE, QUANTITY_SCALE_DIGITS);
    }

    Quantity& operator-=(const Quantity& other) {
        ticks -= other.ticks;
        return *this;
    }
    Quantity& operator+=(const Quantity& other) {
        ticks += other.ticks;
        return *this;
    }
    friend Quantity operator-(Quantity a, const Quantity& b) {
        a -= b;
        return a;
    }
    friend Quantity operator+(Quantity a, const Quantity& b) {
        a += b;
        return a;
    }
};
