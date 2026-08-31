import QRCodeEncoder from "qrcode";

const matrixPath = (value: string) => {
  // The protected centre mark obscures a small part of the matrix. Level H keeps the payload
  // recoverable even with that deliberate obstruction and ordinary camera perspective loss.
  const matrix = QRCodeEncoder.create(value, { errorCorrectionLevel: "H" }).modules;
  const commands: string[] = [];

  for (let row = 0; row < matrix.size; row += 1) {
    let column = 0;
    while (column < matrix.size) {
      while (column < matrix.size && matrix.get(row, column) === 0) column += 1;
      const start = column;
      while (column < matrix.size && matrix.get(row, column) !== 0) column += 1;
      if (column > start) commands.push(`M${start + 4} ${row + 4}h${column - start}v1H${start + 4}z`);
    }
  }

  return { commands: commands.join(""), dimension: matrix.size + 8 };
};

export { matrixPath };
