// Package histwriter contains functions for writing HdrHistograms in a readable
// format consumable by http://hdrhistogram.github.io/HdrHistogram/plotFiles.html.
package histwriter

import (
	"fmt"
	"io"
	"os"

	"github.com/codahale/hdrhistogram"
)

// WriteDistribution writes the percentile distribution of a Histogram in a
// format plottable by http://hdrhistogram.github.io/HdrHistogram/plotFiles.html
// to the given Writer. Percentiles is a list of percentiles to include, e.g.
// 10.0, 50.0, 99.0, 99.99, etc. If percentiles is nil, it defaults to a
// logarithmic percentile scale. The scaleFactor is used to scale values.
func WriteDistribution(hist *hdrhistogram.Histogram, percentiles Percentiles, scaleFactor float64, writer io.Writer) error {
	if percentiles == nil {
		percentiles = Logarithmic
	}

	if _, err := writer.Write([]byte("Value    Percentile    TotalCount    1/(1-Percentile)\n\n")); err != nil {
		return err
	}

	totalCount := hist.TotalCount()
	for _, percentile := range percentiles {
		value := float64(hist.ValueAtQuantile(percentile)) * scaleFactor
		oneByPercentile := getOneByPercentile(percentile)
		countAtPercentile := int64(((percentile / 100) * float64(totalCount)) + 0.5)
		_, err := writer.Write([]byte(fmt.Sprintf("%f    %f        %d            %f\n",
			value, percentile/100, countAtPercentile, oneByPercentile)))
		if err != nil {
			return err
		}
	}

	return nil
}

// WriteDistributionFile writes the percentile distribution of a Histogram in a
// format plottable by http://hdrhistogram.github.io/HdrHistogram/plotFiles.html
// to the given file. If the file doesn't exist, it's created. Percentiles is a
// list of percentiles to include, e.g.  10.0, 50.0, 99.0, 99.99, etc. If
// percentiles is nil, it defaults to a logarithmic percentile scale. The
// scaleFactor is used to scale values.
func WriteDistributionFile(hist *hdrhistogram.Histogram, percentiles Percentiles, scaleFactor float64, file string) error {
	f, err := os.Open(file)
	if err != nil {
		f, err = os.Create(file)
		if err != nil {
			return err
		}
	}
	defer f.Close()

	return WriteDistribution(hist, percentiles, scaleFactor, f)
}

func getOneByPercentile(percentile float64) float64 {
	if percentile < 100 {
		return 1 / (1 - (percentile / 100))
	}
	return float64(10000000)
}
