package edeka

import "math/rand/v2"

// generateIMEI returns a 15-digit IMEI string with a valid Luhn check digit.
func generateIMEI() string {
	var buf [15]byte
	for i := 0; i < 14; i++ {
		buf[i] = '0' + byte(rand.IntN(10))
	}

	// Luhn check digit (ISO/IEC 7812): starting from the digit immediately left
	// of the (future) check digit, double every second digit moving leftward.
	// For the 14 payload digits at positions 0-13, the doubled positions are the
	// odd ones (1, 3, 5, 7, 9, 11, 13) so that the final 15-digit IMEI validates.
	sum := 0
	for i := 13; i >= 0; i-- {
		d := int(buf[i] - '0')
		if i%2 == 1 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	buf[14] = '0' + byte((10-(sum%10))%10)
	return string(buf[:])
}

// getRandomDevice picks a manufacturer/model pair from the hard-coded device map.
func getRandomDevice() (string, string) {
	manufacturer := manufacturers[rand.IntN(len(manufacturers))]
	devices := deviceMap[manufacturer]
	return manufacturer, devices[rand.IntN(len(devices))]
}
