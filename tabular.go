package main

import (
	"encoding/csv"
	"io"
)

const (
	ROWS_TO_READ = 100
)

type Record []string

type TabularReader struct {
	headers  Record
	nextRows []Record
	reader   *csv.Reader
}

func NewTabularReader(ioReader io.ReadSeeker) (reader *TabularReader, err error) {
	// We will attempt both comma separated and tab separated.  We
	// prefer the one with no errors.  If neither has errors, we
	// prefer the one with more fields.  If they have the same number
	// of fields, we prefer comma separated.

	// If one has errors, we only allow the other one if the error was
	// a field count error and there is more than one field for the other.

	// figure out where we are starting
	start, err := ioReader.Seek(0, 1)
	if err != nil {
		return nil, err
	}
	csvReader, csvErr := newTabularReader(ioReader, ',')

	// remember where the csv reader ended in case we use this one (we
	// will need to rewind to this point in that case)
	csvEnd, err := ioReader.Seek(0, 1)
	if err != nil {
		return nil, err
	}

	_, err = ioReader.Seek(start, 0)
	if err != nil {
		return nil, err
	}

	tsvReader, tsvErr := newTabularReader(ioReader, '\t')

	switch {
	case csvErr != nil && tsvErr != nil:
		// no valid option
		return nil, csvErr
	case csvErr == nil && tsvErr != nil:
		if !isFieldCountErr(tsvErr) || csvReader.reader.FieldsPerRecord == 1 {
			return nil, tsvErr
		}
		_, err = ioReader.Seek(csvEnd, 0)
		if err != nil {
			return nil, err
		}
		return csvReader, nil
	case csvErr != nil && tsvErr == nil:
		if !isFieldCountErr(csvErr) || tsvReader.reader.FieldsPerRecord == 1 {
			return nil, csvErr
		}
		return tsvReader, nil
	default:
		// neither had an error, look for the one with more fields.
		// If the field count is the same, default to csv
		if csvReader.reader.FieldsPerRecord >= tsvReader.reader.FieldsPerRecord {
			_, err = ioReader.Seek(csvEnd, 0)
			if err != nil {
				return nil, err
			}
			return csvReader, nil
		} else {
			return tsvReader, nil
		}
	}

	return
}

func isFieldCountErr(err error) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*csv.ParseError)
	if !ok {
		return false
	}
	return e.Err == csv.ErrFieldCount
}

func newTabularReader(ioReader io.Reader, fieldSeparator rune) (reader *TabularReader, err error) {
	csvReader := csv.NewReader(ioReader)
	csvReader.Comma = fieldSeparator
	headers, err := csvReader.Read()
	if err != nil {
		return nil, err
	}
	initialRows := make([]Record, 0, ROWS_TO_READ)
	for i := 0; i < ROWS_TO_READ; i++ {
		nextRow, err := csvReader.Read()
		switch {
		case err == io.EOF:
			break
		case err != nil:
			return nil, err
		default:
			initialRows = append(initialRows, nextRow)
		}
	}
	reader = &TabularReader{headers, initialRows, csvReader}
	return reader, nil
}

func (r *TabularReader) Read() (record map[string]string, err error) {
	var nextLine []string
	if len(r.nextRows) == 0 {
		nextLine, err = r.reader.Read()
		if err != nil {
			return nil, err
		}
	} else {
		nextLine = r.nextRows[0]
		r.nextRows = r.nextRows[1:len(r.nextRows)]
	}
	record = make(map[string]string)
	for i, header := range r.headers {
		record[header] = nextLine[i]
	}
	return record, nil
}
