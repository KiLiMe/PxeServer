package config

import (
	"encoding/hex"
	"errors"
	"net"
	"strconv"
	"strings"

	"pxeserv/internal/locales"
)

// ParseExtraOptions 解析“额外 DHCP 选项”文本，返回 选项号 -> 原始字节。
//
// 语法：多项用分号（或逗号、换行）分隔，每项为 code[.sub]=值。值支持：
//
//	ip:192.168.5.2  或直接写 IPv4  -> 4 字节
//	str:abc                        -> ASCII
//	hex:0102ff                     -> 原始十六进制
//	u8:3 / u16:3600 / u32:60       -> 大端整数
//
// 带 .sub 时按“子选项”编码：数据 = 子选项号 + 长度 + 值，同一 code 的多个子项自动拼接。
// 例：175.6=ip:1.8.1.1; 175.8=ip:192.168.5.1; 60=str:PXEClient
func (c *Config) ParseExtraOptions() (map[byte][]byte, error) {
	out := make(map[byte][]byte)
	if strings.TrimSpace(c.ExtraOptions) == "" {
		return out, nil
	}
	items := strings.FieldsFunc(c.ExtraOptions, func(r rune) bool {
		return r == ';' || r == '\n' || r == '\r'
	})
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		kv := strings.SplitN(item, "=", 2)
		if len(kv) != 2 {
			return nil, errors.New(locales.ErrExtraMissingEq(item))
		}
		key := strings.TrimSpace(kv[0])
		valText := strings.TrimSpace(kv[1])

		parts := strings.Split(key, ".")
		if len(parts) > 2 {
			return nil, errors.New(locales.ErrExtraFormat(item))
		}
		code64, err := strconv.Atoi(parts[0])
		if err != nil || code64 <= 0 || code64 >= 255 {
			return nil, errors.New(locales.ErrExtraCodeRange(item))
		}
		code := byte(code64)
		switch code {
		case 53, 54, 52:
			return nil, errors.New(locales.ErrExtraReserved(item, int(code)))
		}

		data, err := decodeOptionValue(valText)
		if err != nil {
			return nil, errors.New(locales.ErrExtraInvalidValue(item, err.Error()))
		}
		if len(parts) == 2 {
			sub64, err := strconv.Atoi(parts[1])
			if err != nil || sub64 <= 0 || sub64 >= 255 {
				return nil, errors.New(locales.ErrExtraSubRange(item))
			}
			if len(data) > 255 {
				return nil, errors.New(locales.ErrExtraTooLong(item))
			}
			data = append([]byte{byte(sub64), byte(len(data))}, data...)
		}
		if len(data) > 255 {
			return nil, errors.New(locales.ErrExtraTooLong(item))
		}
		out[code] = append(out[code], data...)
	}
	return out, nil
}

func decodeOptionValue(text string) ([]byte, error) {
	lower := strings.ToLower(text)
	switch {
	case strings.HasPrefix(lower, "ip:"):
		ip := net.ParseIP(strings.TrimSpace(text[3:])).To4()
		if ip == nil {
			return nil, errors.New(locales.ErrValueNotIPv4(text))
		}
		return ip, nil
	case strings.HasPrefix(lower, "str:"):
		return []byte(text[4:]), nil
	case strings.HasPrefix(lower, "hex:"):
		raw := strings.Map(func(r rune) rune {
			if r == ' ' || r == '-' {
				return -1
			}
			return r
		}, text[4:])
		b, err := hex.DecodeString(raw)
		if err != nil {
			return nil, errors.New(locales.ErrHexInvalid(text[4:]))
		}
		return b, nil
	case strings.HasPrefix(lower, "u8:"):
		return bigEndianValue(text[3:], 1, text)
	case strings.HasPrefix(lower, "u16:"):
		return bigEndianValue(text[4:], 2, text)
	case strings.HasPrefix(lower, "u32:"):
		return bigEndianValue(text[4:], 4, text)
	}
	if ip := net.ParseIP(text).To4(); ip != nil {
		return ip, nil
	}
	// 没写前缀也不是 IPv4，按字符串处理（如 60=str 之外的厂商串）。
	return []byte(text), nil
}

func bigEndianValue(numText string, size int, whole string) ([]byte, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(numText), 10, 64)
	if err != nil {
		return nil, errors.New(locales.ErrNumberInvalid(whole))
	}
	out := make([]byte, size)
	for i := size - 1; i >= 0; i-- {
		out[i] = byte(n)
		n >>= 8
	}
	if n != 0 {
		return nil, errors.New(locales.ErrNumberOverflow(whole, size))
	}
	return out, nil
}
